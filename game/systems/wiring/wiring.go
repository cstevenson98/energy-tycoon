// Package wiring joins freshly placed grid entities into ElectricalNetwork
// (and tears them out on delete). Placement owns input/occupancy; this package
// owns bus/branch/history mutations so placement stays input-focused.
//
// Lines have no bus: AttachLine adds a series branch between the buses at the
// path endpoints (recorded on LineEndpoints). Generator/house ghost ports all
// resolve to that device's single bus — they never create extra buses.
package wiring

import (
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/gowasm-engine/pkg/ecs"
	"github.com/cstevenson98/gowasm-engine/pkg/logger"
)

// Attach registers entity e as a bus (generator, house, or junction), stamps
// NetworkLink + BusHistory, and writes house PQ when applicable.
// Device ghost ports do not get their own buses; lines snap to the device bus
// via ResolveBus. No automatic contact shorts.
func Attach(w *ecs.World, e ecs.Entity, kind grid.Tool, cell grid.GridCoord, occupancy *grid.GridOccupancy) {
	if kind == grid.ToolLine {
		AttachLine(w, e, occupancy)
		return
	}

	net := ecs.GetResource[network.ElectricalNetwork](w)
	if net == nil {
		return
	}
	bus, err := net.AddBus(e, toolToBusType(kind))
	if err != nil {
		logger.Logger.Errorf("grid-sim: Attach AddBus: %v", err)
		return
	}
	ecs.NewMap1[network.NetworkLink](w).Add(e, &network.NetworkLink{BusID: bus.ID})
	h := network.NewBusHistory()
	ecs.NewMap1[network.BusHistory](w).Add(e, &h)

	if kind == grid.ToolHouse {
		if hl := ecs.NewMap1[grid.HouseLoad](w).Get(e); hl != nil {
			net.SetBusSpec(bus.ID, network.PQSpec(-hl.PKw*1000, -hl.QKw*1000))
		}
	}
}

// ResolveBus returns the network bus for a grid cell:
//   - cell occupied by gen/house/junction → that entity's bus
//   - empty ghost port of a gen/house → that device's bus (all 4 ports share it)
//   - otherwise → false
func ResolveBus(w *ecs.World, occupancy *grid.GridOccupancy, cell grid.GridCoord) (*network.Bus, bool) {
	net := ecs.GetResource[network.ElectricalNetwork](w)
	if net == nil || occupancy == nil {
		return nil, false
	}
	if e, ok := occupancy.Cells[cell]; ok {
		go_ := ecs.NewMap1[grid.GridObject](w).Get(e)
		if go_ == nil || go_.Kind == grid.ToolLine {
			return nil, false
		}
		return net.BusForEntity(e)
	}
	if host, _, ok := grid.DevicePortHost(w, occupancy, cell); ok {
		return net.BusForEntity(host)
	}
	return nil, false
}

// HasBusAt reports whether ResolveBus would succeed for cell.
func HasBusAt(w *ecs.World, occupancy *grid.GridOccupancy, cell grid.GridCoord) bool {
	_, ok := ResolveBus(w, occupancy, cell)
	return ok
}

// AttachLine wires a polyline as a series branch between the buses resolved
// at path[0] and path[len-1] (occupied bus cell or device ghost port).
func AttachLine(w *ecs.World, e ecs.Entity, occupancy *grid.GridOccupancy) {
	net := ecs.GetResource[network.ElectricalNetwork](w)
	if net == nil {
		return
	}
	lp := ecs.NewMap1[grid.LinePath](w).Get(e)
	if lp == nil || len(lp.Cells) == 0 {
		logger.Logger.Errorf("grid-sim: AttachLine: missing LinePath")
		return
	}
	start, end := lp.Cells[0], lp.Cells[len(lp.Cells)-1]
	fromBus, ok := ResolveBus(w, occupancy, start)
	if !ok {
		logger.Logger.Errorf("grid-sim: AttachLine: no bus at start %+v", start)
		return
	}
	toBus, ok := ResolveBus(w, occupancy, end)
	if !ok {
		logger.Logger.Errorf("grid-sim: AttachLine: no bus at end %+v", end)
		return
	}
	if fromBus.ID == toBus.ID {
		logger.Logger.Debugf("grid-sim: AttachLine: same bus at both ends, skip branch")
		return
	}

	var r, x float64
	if lsp := ecs.NewMap1[grid.LineSegmentProps](w).Get(e); lsp != nil {
		r, x = lsp.ResistanceOhm, lsp.ReactanceOhm
	}
	br := net.AddBranch(fromBus.ID, toBus.ID, r, x)
	if ep := ecs.NewMap1[grid.LineEndpoints](w).Get(e); ep != nil {
		ep.FromBus = uint64(fromBus.ID)
		ep.ToBus = uint64(toBus.ID)
		ep.BranchID = uint64(br.ID)
		ep.Wired = true
	}
}

// Detach removes the entity's contribution to the network: bus (and incident
// branches) for gen/house/junction, or the recorded series branch for a line.
func Detach(w *ecs.World, e ecs.Entity) {
	net := ecs.GetResource[network.ElectricalNetwork](w)
	if net == nil {
		return
	}
	if ep := ecs.NewMap1[grid.LineEndpoints](w).Get(e); ep != nil && ep.Wired {
		net.RemoveBranch(network.BranchID(ep.BranchID))
		ep.Wired = false
		return
	}
	if bus, ok := net.BusForEntity(e); ok {
		net.RemoveBus(bus.ID)
	}
}

func toolToBusType(t grid.Tool) network.BusType {
	switch t {
	case grid.ToolGenerator:
		return network.BusGenerator
	case grid.ToolHouse:
		return network.BusLoad
	case grid.ToolJunction:
		return network.BusJunction
	default:
		return network.BusJunction
	}
}
