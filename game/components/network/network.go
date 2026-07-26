package network

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/cstevenson98/gowasm-engine/pkg/ecs"
	"github.com/cstevenson98/gowasm-engine/pkg/logger"
)

// ErrDuplicateBus is returned by AddBus when the entity already has a bus.
var ErrDuplicateBus = errors.New("network: entity already has a bus")

// BusType distinguishes how a bus participates in power flow.
type BusType int

const (
	BusGenerator BusType = iota // slack / PV — power source
	BusLoad                     // PQ — power consumer (house)
	BusJunction                 // passive node (line segment)
)

// BusID uniquely identifies a bus within an ElectricalNetwork.
type BusID uint64

// BranchID uniquely identifies a branch within an ElectricalNetwork.
type BranchID uint64

// Bus is a node in the electrical network. It carries a back-link to the ECS
// entity it represents on the grid. Electrical quantities (voltage magnitude
// and angle, active/reactive power injection) are natural additions here when
// power-flow is implemented.
type Bus struct {
	ID     BusID
	Type   BusType
	Entity ecs.Entity
}

// Branch is an undirected connection between two buses representing a grid
// adjacency (line segment, or direct generator/house contact).
type Branch struct {
	ID         BranchID
	From       BusID
	To         BusID
	Resistance float64 // series resistance R [Ω]; 0 for direct connections
	Reactance  float64 // series reactance X [Ω]; 0 for purely resistive links
}

// NetworkLink is the ECS component that is the grid-side half of the join.
// Any entity that participates in the electrical network carries this so that
// ECS filters (e.g. Filter2[GridObject, NetworkLink]) can reach it without
// going through the network resource.
type NetworkLink struct {
	BusID BusID
}

// ElectricalNetwork is a per-World resource representing the power grid graph.
// It is the single source of truth for network topology and operating state.
// ECS entities hold a NetworkLink back-reference and a BusHistory component
// for the last N solve samples (see history.go).
//
// Dirty is set by every topology/spec mutation (AddBus, RemoveBus, AddBranch,
// RemoveBranch, SetBusSpec). LoadflowSystem clears it after a successful
// attempt to re-solve, so the Newton-Raphson pass only runs when the circuit
// has actually changed.
type ElectricalNetwork struct {
	buses      map[BusID]*Bus
	branches   map[BranchID]*Branch
	entityBus  map[ecs.Entity]BusID // join: grid entity → bus
	incidence  map[BusID][]BranchID // adjacency list for fast Neighbors()
	nextBus    BusID
	nextBranch BranchID

	// State holds the current operating-point snapshot. It is kept in sync
	// with the topology: entries are created/removed with buses and branches.
	State *StaticState

	// Dirty is true when State may be stale relative to the current graph/specs.
	Dirty bool
}

// NewElectricalNetwork creates an empty network with an initialised StaticState.
func NewElectricalNetwork() *ElectricalNetwork {
	return &ElectricalNetwork{
		buses:     make(map[BusID]*Bus),
		branches:  make(map[BranchID]*Branch),
		entityBus: make(map[ecs.Entity]BusID),
		incidence: make(map[BusID][]BranchID),
		State:     newStaticState(),
	}
}

// MarkDirty flags the network so LoadflowSystem will re-solve next frame.
func (n *ElectricalNetwork) MarkDirty() { n.Dirty = true }

// ClearDirty acknowledges that State has been refreshed for the current graph.
func (n *ElectricalNetwork) ClearDirty() { n.Dirty = false }

// --- Join interface ---------------------------------------------------------

// AddBus registers a new bus for the given ECS entity and returns it.
// Returns ErrDuplicateBus if the entity already has a bus.
func (n *ElectricalNetwork) AddBus(e ecs.Entity, t BusType) (*Bus, error) {
	if _, exists := n.entityBus[e]; exists {
		return nil, ErrDuplicateBus
	}
	id := n.nextBus
	n.nextBus++
	b := &Bus{ID: id, Type: t, Entity: e}
	n.buses[id] = b
	n.entityBus[e] = id
	n.incidence[id] = nil
	n.State.Buses[id] = &BusState{Spec: defaultBusSpec(t)}
	n.MarkDirty()
	return b, nil
}

// BusForEntity returns the bus linked to the given ECS entity, or false if the
// entity has no bus (e.g. it is a blank tile).
func (n *ElectricalNetwork) BusForEntity(e ecs.Entity) (*Bus, bool) {
	id, ok := n.entityBus[e]
	if !ok {
		return nil, false
	}
	return n.buses[id], true
}

// Bus returns a bus by ID.
func (n *ElectricalNetwork) Bus(id BusID) (*Bus, bool) {
	b, ok := n.buses[id]
	return b, ok
}

// RemoveBus removes a bus and all of its incident branches from the network.
func (n *ElectricalNetwork) RemoveBus(id BusID) {
	for _, brID := range n.incidence[id] {
		br, ok := n.branches[brID]
		if !ok {
			continue
		}
		other := br.To
		if other == id {
			other = br.From
		}
		n.incidence[other] = removeBranchID(n.incidence[other], brID)
		delete(n.branches, brID)
		delete(n.State.Branches, brID)
	}
	if b, ok := n.buses[id]; ok {
		delete(n.entityBus, b.Entity)
	}
	delete(n.buses, id)
	delete(n.incidence, id)
	delete(n.State.Buses, id)
	n.MarkDirty()
}

// --- Graph operations -------------------------------------------------------

// AddBranch connects two buses with series impedance r+jx (ohms) and returns
// the new branch. The connection is undirected: both buses' incidence lists
// are updated. Pass r=0, x=0 for a direct (near-lossless) contact; r is
// clamped to minResistance when building the Y-bus.
func (n *ElectricalNetwork) AddBranch(from, to BusID, r, x float64) *Branch {
	id := n.nextBranch
	n.nextBranch++
	br := &Branch{ID: id, From: from, To: to, Resistance: r, Reactance: x}
	n.branches[id] = br
	n.incidence[from] = append(n.incidence[from], id)
	n.incidence[to] = append(n.incidence[to], id)
	n.State.Branches[id] = &BranchState{History: NewBranchHistory()}
	n.MarkDirty()
	return br
}

// RemoveBranch removes a branch from the network.
func (n *ElectricalNetwork) RemoveBranch(id BranchID) {
	br, ok := n.branches[id]
	if !ok {
		return
	}
	n.incidence[br.From] = removeBranchID(n.incidence[br.From], id)
	n.incidence[br.To] = removeBranchID(n.incidence[br.To], id)
	delete(n.branches, id)
	delete(n.State.Branches, id)
	n.MarkDirty()
}

// Neighbors returns all buses directly connected to the given bus.
func (n *ElectricalNetwork) Neighbors(id BusID) []*Bus {
	var result []*Bus
	for _, brID := range n.incidence[id] {
		br := n.branches[brID]
		other := br.To
		if other == id {
			other = br.From
		}
		if b, ok := n.buses[other]; ok {
			result = append(result, b)
		}
	}
	return result
}

func removeBranchID(s []BranchID, id BranchID) []BranchID {
	for i, v := range s {
		if v == id {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

// --- State API (entity → solver boundary conditions, solver → caller results) ---

// SetBusSpec sets the boundary conditions for a bus. Called by any system
// that reads entity components and needs to inject known quantities into the
// solver (e.g. a generator entity sets its terminal voltage, a load entity
// sets its P/Q demand). It is a no-op if the bus ID is unknown.
func (n *ElectricalNetwork) SetBusSpec(id BusID, spec BusSpec) {
	if bs, ok := n.State.Buses[id]; ok {
		bs.Spec = spec
		n.MarkDirty()
	}
}

// BusStateFor returns the full state (Spec + Result) for a bus, or false if
// the bus ID is unknown.
func (n *ElectricalNetwork) BusStateFor(id BusID) (*BusState, bool) {
	bs, ok := n.State.Buses[id]
	return bs, ok
}

// BranchStateFor returns the full state (Result) for a branch, or false if
// the branch ID is unknown.
func (n *ElectricalNetwork) BranchStateFor(id BranchID) (*BranchState, bool) {
	br, ok := n.State.Branches[id]
	return br, ok
}

// --- Read access for simulation systems ------------------------------------

// Buses returns the full bus map. Callers must not mutate the returned map.
func (n *ElectricalNetwork) Buses() map[BusID]*Bus {
	return n.buses
}

// Branches returns the full branch map. Callers must not mutate the returned map.
func (n *ElectricalNetwork) Branches() map[BranchID]*Branch {
	return n.branches
}

// Print prints a human-readable summary of all buses and branches to stdout,
// sorted by ID so the output is stable.
func (n *ElectricalNetwork) Print() {
	busIDs := make([]int, 0, len(n.buses))
	for id := range n.buses {
		busIDs = append(busIDs, int(id))
	}
	sort.Ints(busIDs)

	fmt.Printf("ElectricalNetwork: %d bus(es), %d branch(es)\n", len(n.buses), len(n.branches))
	fmt.Println("Buses:")
	for _, raw := range busIDs {
		b := n.buses[BusID(raw)]
		fmt.Printf("  bus %d  type=%-10s entity=%v\n", b.ID, b.Type, b.Entity)
	}

	branchIDs := make([]int, 0, len(n.branches))
	for id := range n.branches {
		branchIDs = append(branchIDs, int(id))
	}
	sort.Ints(branchIDs)

	fmt.Println("Branches:")
	for _, raw := range branchIDs {
		br := n.branches[BranchID(raw)]
		fmt.Printf("  branch %d  %d — %d\n", br.ID, br.From, br.To)
	}
}

// Log writes the same summary as Print but routes it through the engine
// logger at INFO level so it appears alongside other game log output.
func (n *ElectricalNetwork) Log() {
	busIDs := make([]int, 0, len(n.buses))
	for id := range n.buses {
		busIDs = append(busIDs, int(id))
	}
	sort.Ints(busIDs)

	branchIDs := make([]int, 0, len(n.branches))
	for id := range n.branches {
		branchIDs = append(branchIDs, int(id))
	}
	sort.Ints(branchIDs)

	var sb strings.Builder
	fmt.Fprintf(&sb, "network: %d bus(es), %d branch(es) |", len(n.buses), len(n.branches))
	for _, raw := range busIDs {
		b := n.buses[BusID(raw)]
		fmt.Fprintf(&sb, " bus%d(%s)", b.ID, b.Type)
	}
	sb.WriteString(" |")
	for _, raw := range branchIDs {
		br := n.branches[BranchID(raw)]
		fmt.Fprintf(&sb, " %d-%d", br.From, br.To)
	}
	logger.Logger.Info(sb.String())
}

// LogVoltages logs the solved voltage magnitude and angle at every bus,
// sorted by ID, at INFO level — along with whether the last solve converged
// and how many iterations it took. Values reflect whatever is currently in
// n.State.Buses[*].Result, so on a failed/non-converged solve this still
// prints the best available (possibly flat-start or stale) estimate.
// Voltages are in volts (LV, see NominalVoltageV); P is logged in kW.
func (n *ElectricalNetwork) LogVoltages() {
	busIDs := make([]int, 0, len(n.buses))
	for id := range n.buses {
		busIDs = append(busIDs, int(id))
	}
	sort.Ints(busIDs)

	var sb strings.Builder
	fmt.Fprintf(&sb, "loadflow: converged=%v iterations=%d |", n.State.Converged, n.State.Iterations)
	for _, raw := range busIDs {
		id := BusID(raw)
		b := n.buses[id]
		res := n.State.Buses[id].Result
		fmt.Fprintf(&sb, " bus%d(%s)=%.1fV∠%.2f° P=%.2fkW",
			b.ID, b.Type, res.VoltMag, res.VoltAng*180/math.Pi, res.PInject/1000)
	}
	logger.Logger.Info(sb.String())
}

func (t BusType) String() string {
	switch t {
	case BusGenerator:
		return "Generator"
	case BusLoad:
		return "Load"
	case BusJunction:
		return "Junction"
	default:
		return fmt.Sprintf("BusType(%d)", int(t))
	}
}
