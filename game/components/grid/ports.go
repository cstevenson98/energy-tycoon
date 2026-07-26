package grid

import "github.com/cstevenson98/gowasm-engine/pkg/ecs"

// DevicePortHost is a generator or house that owns cardinal ghost ports.
// All four empty cardinal neighbours snap to that host's single network bus.
func DevicePortHost(w *ecs.World, occupancy *GridOccupancy, cell GridCoord) (host ecs.Entity, kind Tool, ok bool) {
	if occupancy == nil || occupancy.Occupied(cell) {
		return ecs.Entity{}, ToolNone, false
	}
	for _, nb := range CardinalNeighbours(cell) {
		e, exists := occupancy.Cells[nb]
		if !exists {
			continue
		}
		go_ := ecs.NewMap1[GridObject](w).Get(e)
		if go_ == nil {
			continue
		}
		if go_.Kind == ToolGenerator || go_.Kind == ToolHouse {
			return e, go_.Kind, true
		}
	}
	return ecs.Entity{}, ToolNone, false
}

// IsDeviceGhostPort reports whether cell is an empty cardinal neighbour of a
// generator or house (snap target sharing that device's bus).
func IsDeviceGhostPort(w *ecs.World, occupancy *GridOccupancy, cell GridCoord) bool {
	_, _, ok := DevicePortHost(w, occupancy, cell)
	return ok
}
