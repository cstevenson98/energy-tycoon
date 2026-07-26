// Package placement implements PlacementSystem: toolbar tool selection and
// grid spawn/delete/line placement. Hover and ToolNone selection live in
// package pointer; electrical join/leave is delegated to package wiring.
package placement

import (
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/energy-tycoon/game/systems/wiring"
	"github.com/cstevenson98/milo/pkg/components"
	"github.com/cstevenson98/milo/pkg/ecs"
	"github.com/cstevenson98/milo/pkg/logger"
)

// PlacementSystem turns toolbar clicks and active-tool grid clicks into
// spawn/delete/line actions. It mutates PlacementState.Tool / Line* and
// GridOccupancy; pointer owns hover/selection.
type PlacementSystem struct{}

// NewPlacementSystem builds the system.
func NewPlacementSystem(_ *ecs.World) *PlacementSystem {
	return &PlacementSystem{}
}

// Update handles toolbar clicks and placement when a tool is active.
func (s *PlacementSystem) Update(w *ecs.World, _ float64) {
	in := ecs.GetResource[components.Input](w)
	placement := ecs.GetResource[grid.PlacementState](w)
	occupancy := ecs.GetResource[grid.GridOccupancy](w)
	cam := ecs.GetResource[components.Camera](w)
	if in == nil || placement == nil || occupancy == nil || cam == nil {
		return
	}

	mouse := in.State.Mouse
	if !mouse.Left.Pressed || mouse.Left.PressedLastFrame {
		return
	}

	bounds := ecs.GetResource[components.ScreenBounds](w)
	if bounds != nil && mouse.X >= gameconfig.Global.PlayfieldWidth(bounds.W) {
		return
	}

	if mouse.Y < gameconfig.Global.ToolbarHeight {
		handleToolbarClick(placement, mouse.X, mouse.Y)
		return
	}

	if placement.Tool == grid.ToolNone {
		return
	}

	s.handleGridClick(w, placement, occupancy, cam, mouse.X, mouse.Y)
}

func handleToolbarClick(placement *grid.PlacementState, x, y float64) {
	for _, b := range grid.ToolbarButtons() {
		if !b.Contains(x, y) {
			continue
		}
		if placement.Tool == b.Tool {
			placement.Tool = grid.ToolNone
		} else {
			placement.Tool = b.Tool
		}
		placement.LinePending = false
		return
	}
}

func (s *PlacementSystem) handleGridClick(w *ecs.World, placement *grid.PlacementState, occupancy *grid.GridOccupancy, cam *components.Camera, x, y float64) {
	cell, ok := grid.ScreenToCell(cam, x, y)
	if !ok {
		return
	}

	switch placement.Tool {
	case grid.ToolGenerator:
		if e, ok := placeSingle(w, occupancy, cell, grid.SpawnGenerator); ok {
			wiring.Attach(w, e, grid.ToolGenerator, cell, occupancy)
		}
	case grid.ToolHouse:
		if e, ok := placeSingle(w, occupancy, cell, grid.SpawnHouse); ok {
			wiring.Attach(w, e, grid.ToolHouse, cell, occupancy)
		}
	case grid.ToolDelete:
		deleteAt(w, occupancy, cell)
	case grid.ToolLine:
		s.handleLineClick(w, placement, occupancy, cell)
	}
}

func placeSingle(w *ecs.World, occupancy *grid.GridOccupancy, cell grid.GridCoord, spawn func(*ecs.World, grid.GridCoord) ecs.Entity) (ecs.Entity, bool) {
	if occupancy.Occupied(cell) {
		logger.Logger.Debugf("grid-sim: cell %+v occupied, ignoring place", cell)
		return ecs.Entity{}, false
	}
	e := spawn(w, cell)
	occupancy.Occupy(cell, e)
	return e, true
}

func (s *PlacementSystem) handleLineClick(w *ecs.World, placement *grid.PlacementState, occupancy *grid.GridOccupancy, cell grid.GridCoord) {
	if !lineClickValid(w, occupancy, cell) {
		logger.Logger.Debugf("grid-sim: invalid line click at %+v", cell)
		return
	}
	if !placement.LinePending {
		placement.LinePending = true
		placement.LineStart = cell
		return
	}
	placement.LinePending = false
	completeLine(w, occupancy, placement.LineStart, cell)
}

// lineClickValid: empty, gen ghost port, junction, house, or on a line (split).
// Generator cells themselves are rejected (use a ghost port).
func lineClickValid(w *ecs.World, occupancy *grid.GridOccupancy, cell grid.GridCoord) bool {
	if e, ok := occupancy.Cells[cell]; ok {
		go_ := ecs.NewMap1[grid.GridObject](w).Get(e)
		if go_ == nil {
			return false
		}
		switch go_.Kind {
		case grid.ToolGenerator:
			return false
		case grid.ToolHouse, grid.ToolJunction, grid.ToolLine:
			return true
		default:
			return false
		}
	}
	return true // empty or will be treated as free/ghost
}

// CompleteLineForTest runs the end-click line completion path (for tests).
func CompleteLineForTest(w *ecs.World, occupancy *grid.GridOccupancy, start, end grid.GridCoord) {
	completeLine(w, occupancy, start, end)
}

func completeLine(w *ecs.World, occupancy *grid.GridOccupancy, start, end grid.GridCoord) {
	path := grid.ManhattanPath(start, end)
	if len(path) < 2 {
		return
	}

	// Reject path cells blocked by gen (not endpoint attach) or foreign non-line.
	for i, c := range path {
		e, ok := occupancy.Cells[c]
		if !ok {
			continue
		}
		go_ := ecs.NewMap1[grid.GridObject](w).Get(e)
		if go_ == nil {
			continue
		}
		switch go_.Kind {
		case grid.ToolLine:
			// split later
		case grid.ToolHouse, grid.ToolJunction:
			if i != 0 && i != len(path)-1 {
				logger.Logger.Debugf("grid-sim: path blocked by %s at %+v", go_.Kind.KindLabel(), c)
				return
			}
		case grid.ToolGenerator:
			logger.Logger.Debugf("grid-sim: path through generator at %+v", c)
			return
		default:
			return
		}
	}

	// Split existing lines at intersection cells (end-click action).
	type splitReq struct {
		line ecs.Entity
		cell grid.GridCoord
	}
	var splits []splitReq
	seenLine := make(map[ecs.Entity]bool)
	for _, c := range path {
		e, ok := occupancy.Cells[c]
		if !ok {
			continue
		}
		go_ := ecs.NewMap1[grid.GridObject](w).Get(e)
		if go_ == nil || go_.Kind != grid.ToolLine {
			continue
		}
		if seenLine[e] {
			continue
		}
		// Only split if c is an interior cell of that line.
		lp := ecs.NewMap1[grid.LinePath](w).Get(e)
		if lp == nil {
			continue
		}
		if _, _, ok := grid.SplitPathAt(lp.Cells, c); !ok {
			continue
		}
		seenLine[e] = true
		splits = append(splits, splitReq{line: e, cell: c})
	}
	for _, sp := range splits {
		if err := splitLineAt(w, occupancy, sp.line, sp.cell); err != nil {
			logger.Logger.Errorf("grid-sim: splitLineAt: %v", err)
			return
		}
	}

	// Ensure attach points at original start/end (and any new mid junctions
	// already exist from splits).
	if !ensureAttachPoint(w, occupancy, start) {
		return
	}
	if !ensureAttachPoint(w, occupancy, end) {
		return
	}

	// Segment path at every bus-bearing cell (junction/house).
	anchors := collectAnchors(w, occupancy, path)
	if len(anchors) < 2 {
		logger.Logger.Debugf("grid-sim: need ≥2 anchors along path")
		return
	}
	for i := 0; i < len(anchors)-1; i++ {
		a, b := anchors[i], anchors[i+1]
		if a == b {
			continue
		}
		seg := subpath(path, a, b)
		if len(seg) < 2 {
			continue
		}
		placeLineSegment(w, occupancy, seg)
	}
}

func collectAnchors(w *ecs.World, occupancy *grid.GridOccupancy, path []grid.GridCoord) []grid.GridCoord {
	var out []grid.GridCoord
	for _, c := range path {
		// Occupied bus cell, or empty gen/house ghost port (shares device bus).
		if wiring.HasBusAt(w, occupancy, c) {
			out = append(out, c)
		}
	}
	return out
}

func subpath(path []grid.GridCoord, from, to grid.GridCoord) []grid.GridCoord {
	i0, i1 := -1, -1
	for i, c := range path {
		if c == from {
			i0 = i
		}
		if c == to {
			i1 = i
		}
	}
	if i0 < 0 || i1 < 0 || i1 <= i0 {
		return nil
	}
	return append([]grid.GridCoord(nil), path[i0:i1+1]...)
}

func placeLineSegment(w *ecs.World, occupancy *grid.GridOccupancy, seg []grid.GridCoord) {
	// Interior cells must be free (or already this will be new).
	for i := 1; i < len(seg)-1; i++ {
		if occupancy.Occupied(seg[i]) {
			logger.Logger.Debugf("grid-sim: interior %+v occupied, skip segment", seg[i])
			return
		}
	}
	e := grid.SpawnLine(w, seg)
	if e == (ecs.Entity{}) {
		return
	}
	occupyLineInterior(occupancy, e, seg)
	wiring.AttachLine(w, e, occupancy)
}

func occupyLineInterior(occupancy *grid.GridOccupancy, e ecs.Entity, path []grid.GridCoord) {
	for i := 1; i < len(path)-1; i++ {
		occupancy.Occupy(path[i], e)
	}
}

// ensureAttachPoint ensures a line endpoint can resolve to a simulation bus.
// Gen/house ghost ports share the device's single bus (no junction spawned).
// Free empty cells get a real junction bus.
func ensureAttachPoint(w *ecs.World, occupancy *grid.GridOccupancy, cell grid.GridCoord) bool {
	if e, ok := occupancy.Cells[cell]; ok {
		go_ := ecs.NewMap1[grid.GridObject](w).Get(e)
		if go_ == nil {
			return false
		}
		switch go_.Kind {
		case grid.ToolHouse, grid.ToolJunction, grid.ToolGenerator:
			return true
		case grid.ToolLine:
			logger.Logger.Debugf("grid-sim: ensureAttachPoint still line at %+v", cell)
			return false
		default:
			return false
		}
	}
	// Empty ghost port of gen/house → use that device bus; do not spawn.
	if grid.IsDeviceGhostPort(w, occupancy, cell) {
		return true
	}
	// Free endpoint → real junction (its own bus).
	j := grid.SpawnJunction(w, cell)
	occupancy.Occupy(cell, j)
	wiring.Attach(w, j, grid.ToolJunction, cell, occupancy)
	return true
}

// splitLineAt inserts a junction at cell on lineEntity, replaces the line
// with two half-lines sharing that junction.
func splitLineAt(w *ecs.World, occupancy *grid.GridOccupancy, lineEntity ecs.Entity, cell grid.GridCoord) error {
	lp := ecs.NewMap1[grid.LinePath](w).Get(lineEntity)
	if lp == nil {
		return nil
	}
	left, right, ok := grid.SplitPathAt(lp.Cells, cell)
	if !ok {
		return nil
	}

	// Clear old line occupancy and detach branch.
	for i := 1; i < len(lp.Cells)-1; i++ {
		delete(occupancy.Cells, lp.Cells[i])
	}
	wiring.Detach(w, lineEntity)
	w.Remove(lineEntity)

	// Junction at split (may already exist if re-entrant — shouldn't).
	if !occupancy.Occupied(cell) {
		j := grid.SpawnJunction(w, cell)
		occupancy.Occupy(cell, j)
		wiring.Attach(w, j, grid.ToolJunction, cell, occupancy)
	}

	if len(left) >= 2 {
		placeLineSegment(w, occupancy, left)
	}
	if len(right) >= 2 {
		placeLineSegment(w, occupancy, right)
	}
	return nil
}

// deleteAt removes a line (interior hit), or a junction/gen/house and any
// lines that used that cell as an endpoint.
func deleteAt(w *ecs.World, occupancy *grid.GridOccupancy, cell grid.GridCoord) bool {
	e, ok := occupancy.Cells[cell]
	if !ok {
		// Length-2 lines occupy no cells — try hit by endpoint match.
		return deleteLineTouching(w, occupancy, cell)
	}
	go_ := ecs.NewMap1[grid.GridObject](w).Get(e)
	if go_ == nil {
		return false
	}
	switch go_.Kind {
	case grid.ToolLine:
		return deleteLine(w, occupancy, e)
	case grid.ToolJunction, grid.ToolGenerator, grid.ToolHouse:
		return deleteBusEntity(w, occupancy, e, cell)
	default:
		return false
	}
}

func deleteLine(w *ecs.World, occupancy *grid.GridOccupancy, e ecs.Entity) bool {
	if lp := ecs.NewMap1[grid.LinePath](w).Get(e); lp != nil {
		for i := 1; i < len(lp.Cells)-1; i++ {
			delete(occupancy.Cells, lp.Cells[i])
		}
	}
	wiring.Detach(w, e)
	w.Remove(e)
	return true
}

func deleteLineTouching(w *ecs.World, occupancy *grid.GridOccupancy, cell grid.GridCoord) bool {
	f := ecs.NewFilter1[grid.LinePath](w)
	var victim ecs.Entity
	found := false
	f.Each(func(e ecs.Entity, lp *grid.LinePath) {
		if found || lp == nil || len(lp.Cells) == 0 {
			return
		}
		if lp.Cells[0] == cell || lp.Cells[len(lp.Cells)-1] == cell {
			victim = e
			found = true
		}
	})
	if !found {
		logger.Logger.Debugf("grid-sim: delete on empty cell %+v", cell)
		return false
	}
	return deleteLine(w, occupancy, victim)
}

func deleteBusEntity(w *ecs.World, occupancy *grid.GridOccupancy, e ecs.Entity, cell grid.GridCoord) bool {
	// Remove lines that end at this cell.
	f := ecs.NewFilter1[grid.LinePath](w)
	var lines []ecs.Entity
	f.Each(func(le ecs.Entity, lp *grid.LinePath) {
		if lp == nil || len(lp.Cells) == 0 {
			return
		}
		if lp.Cells[0] == cell || lp.Cells[len(lp.Cells)-1] == cell {
			lines = append(lines, le)
		}
	})
	for _, le := range lines {
		deleteLine(w, occupancy, le)
	}
	delete(occupancy.Cells, cell)
	wiring.Detach(w, e)
	w.Remove(e)
	return true
}
