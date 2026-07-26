package network

// NominalVoltageV is the swing-bus voltage magnitude used for a generator
// entity's default spec (see defaultBusSpec). 230V matches typical LV
// (low-voltage) single-phase distribution, so a freshly-placed generator
// with no other configuration acts as a 230V reference for the network.
//
// All electrical quantities in this package are real, physical SI units
// (volts, ohms, watts, amps) rather than a normalised per-unit system: since
// P = V·I and Q, P calculations only ever multiply/divide V by V, R, or G,
// staying in consistent SI units throughout is dimensionally sufficient —
// no base MVA/kV choice is required.
const NominalVoltageV = 230.0

// BusFormulation identifies which quantities are *specified* (known to the
// solver) vs *free* (to be solved) for a bus. It is the discriminant that
// determines which fields of BusSpec are meaningful.
type BusFormulation int

const (
	// Slack is the network reference bus: VoltMag and VoltAng are fixed.
	// P and Q are free — the slack bus absorbs the system's power imbalance.
	// Exactly one bus per connected island must be Slack.
	Slack BusFormulation = iota

	// PV is a generator bus with voltage magnitude control: VoltMag and
	// PInject are fixed. VoltAng and QInject are free.
	PV

	// PQ is a load bus (or passive junction): PInject and QInject are fixed.
	// VoltMag and VoltAng are free. Junctions use PQ with both = 0.
	PQ
)

// String returns a short UI/debug label for the formulation.
func (f BusFormulation) String() string {
	switch f {
	case Slack:
		return "Slack"
	case PV:
		return "PV"
	case PQ:
		return "PQ"
	default:
		return "?"
	}
}

// BusSpec is the boundary condition set by an entity (or a system reading ECS
// components) before each solve. Use the constructor functions — SlackSpec,
// PVSpec, PQSpec, JunctionSpec — rather than constructing this directly, so
// the Formulation discriminant is always consistent with the filled fields.
type BusSpec struct {
	Formulation BusFormulation
	VoltMag     float64 // |V| in volts          — active for Slack + PV
	VoltAng     float64 // angle radians          — active for Slack only
	PInject     float64 // active power, W (+gen) — active for PV + PQ
	QInject     float64 // reactive power, VAR    — active for PQ only
}

// SlackSpec creates a swing-bus spec with the given voltage magnitude and angle.
func SlackSpec(vMag, vAng float64) BusSpec {
	return BusSpec{Formulation: Slack, VoltMag: vMag, VoltAng: vAng}
}

// PVSpec creates a PV-bus spec with the given voltage magnitude and active
// power injection (positive = generation).
func PVSpec(vMag, pInject float64) BusSpec {
	return BusSpec{Formulation: PV, VoltMag: vMag, PInject: pInject}
}

// PQSpec creates a PQ-bus spec with the given active and reactive power
// injections (positive = generation, negative = load).
func PQSpec(pInject, qInject float64) BusSpec {
	return BusSpec{Formulation: PQ, PInject: pInject, QInject: qInject}
}

// JunctionSpec creates a PQ-bus spec with zero injection — the default for
// passive line-segment nodes.
func JunctionSpec() BusSpec {
	return BusSpec{Formulation: PQ}
}

// BusResult holds the quantities written by the solver for one bus after
// convergence. Before the first solve all fields are zero (flat start).
type BusResult struct {
	VoltMag float64 // solved |V| in volts
	VoltAng float64 // solved voltage angle radians
	PInject float64 // solved active power injection, W
	QInject float64 // solved reactive power injection, VAR
}

// BusState is the complete per-bus state: the boundary condition (Spec) set
// by entities, and the solved result (Result) written by the solver.
type BusState struct {
	Spec   BusSpec
	Result BusResult
}

// BranchResult holds the quantities written by the solver for one branch.
// Flows are defined positive in the from→to direction.
type BranchResult struct {
	CurrentMag float64 // |I| in amps
	PFrom      float64 // active power flow from→to, W
	PTo        float64 // active power flow to→from, W
	QFrom      float64 // reactive power flow from→to, VAR
	QTo        float64 // reactive power flow to→from, VAR
}

// BranchState is the complete per-branch state.
type BranchState struct {
	Result  BranchResult
	History BranchHistory // last N |I| samples; see history.go
}

// StaticState is one solved (or partially solved) snapshot of the network:
// bus voltages, injections, and branch flows for a single operating point.
// It is updated in-place by Solver.Solve. Entries mirror the topology —
// they are created and removed automatically with buses and branches.
type StaticState struct {
	Buses      map[BusID]*BusState
	Branches   map[BranchID]*BranchState
	Converged  bool
	Iterations int
	// LastError is set by LoadflowSolver when Solve returns an error
	// (no slack, NR failure, island issues). Empty on success. Shown in ImGui.
	LastError string
}

// newStaticState creates an empty snapshot ready to receive bus and branch entries.
func newStaticState() *StaticState {
	return &StaticState{
		Buses:    make(map[BusID]*BusState),
		Branches: make(map[BranchID]*BranchState),
	}
}

// defaultBusSpec returns a sensible flat-start spec for the given BusType.
// Generators default to Slack (V=NominalVoltageV∠0°); everything else
// defaults to PQ (0,0) — callers (e.g. the placement system, for houses)
// are expected to overwrite this with real P/Q via SetBusSpec once the
// entity's load/generation components are known.
func defaultBusSpec(t BusType) BusSpec {
	if t == BusGenerator {
		return SlackSpec(NominalVoltageV, 0.0)
	}
	return JunctionSpec()
}
