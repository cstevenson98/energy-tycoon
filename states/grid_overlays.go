package states

import (
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/gowasm-engine/pkg/components"
	"github.com/cstevenson98/gowasm-engine/pkg/ecs"
	"github.com/cstevenson98/gowasm-engine/pkg/types"
)

func (s *GridState) renderToolbar() {
	cfg := gameconfig.Global
	ui := s.UI()
	playW := cfg.PlayfieldWidth(s.ScreenWidth())

	ui.Rect(0, 0, playW, cfg.ToolbarHeight, types.Color{0.1, 0.1, 0.12, 1})

	placement := ecs.GetResource[grid.PlacementState](s.World())
	selected := grid.ToolNone
	linePending := false
	if placement != nil {
		selected = placement.Tool
		linePending = placement.LinePending
	}

	for _, b := range grid.ToolbarButtons() {
		bg := types.Color{0.3, 0.3, 0.34, 1}
		fg := types.White
		if b.Tool == selected {
			bg = types.Yellow
			fg = types.Black
		}
		ui.Rect(b.X, b.Y, b.W, b.H, bg)
		ui.TextColored(b.X+3, b.Y+2, fg, b.Label)
	}

	if linePending {
		ui.TextColored(cfg.ButtonMarginX, cfg.ToolbarHeight+4, types.Yellow, "Line: click the end cell")
	} else if selected == grid.ToolNone {
		ui.TextColored(cfg.ButtonMarginX, cfg.ToolbarHeight+4, types.Color{0.7, 0.7, 0.75, 1}, "Click cell to inspect · C clears tool")
	}
}

// renderGridChrome draws the procedural playfield grid, thick polylines,
// junction/ghost circles, hover/selection borders, and the placement ghost.
func (s *GridState) renderGridChrome() {
	placement := ecs.GetResource[grid.PlacementState](s.World())
	cam := ecs.GetResource[components.Camera](s.World())
	occupancy := ecs.GetResource[grid.GridOccupancy](s.World())
	if cam == nil {
		return
	}
	ui := s.UI()

	s.renderGridBackground(ui, cam)
	s.renderPolylines(ui, cam)
	s.renderJunctions(ui, cam)

	if placement == nil {
		return
	}
	if placement.Tool == grid.ToolLine {
		s.renderDeviceGhostPorts(ui, cam, occupancy)
	}
	if placement.HasSelection {
		s.drawCellBorder(ui, cam, placement.SelectedCell, types.Color{0.2, 0.85, 1, 1}, 2)
	}
	if placement.HoverValid {
		s.drawCellBorder(ui, cam, placement.HoverCell, types.Color{1, 1, 0.35, 1}, 2)
	}
	if placement.Tool != grid.ToolNone && placement.HoverValid {
		s.drawPlacementGhost(ui, cam, occupancy, placement)
	}
}

func (s *GridState) renderGridBackground(ui types.UIManager, cam *components.Camera) {
	cfg := gameconfig.Global
	playW := cfg.PlayfieldWidth(s.ScreenWidth())
	screenH := s.ScreenHeight()
	ts := cfg.TileSize
	zoom := cam.Zoom
	if zoom <= 0 {
		zoom = 1
	}

	worldX0 := cam.X
	worldY0 := cam.Y + cfg.ToolbarHeight/zoom
	worldX1 := cam.X + playW/zoom
	worldY1 := cam.Y + screenH/zoom

	c0 := int(worldX0 / ts)
	r0 := int(worldY0 / ts)
	c1 := int(worldX1/ts) + 1
	r1 := int(worldY1/ts) + 1
	if c0 < 0 {
		c0 = 0
	}
	if r0 < 0 {
		r0 = 0
	}
	if c1 > cfg.GridCols {
		c1 = cfg.GridCols
	}
	if r1 > cfg.GridRows {
		r1 = cfg.GridRows
	}
	if c0 > c1 || r0 > r1 {
		return
	}

	line := types.Color{0.22, 0.22, 0.26, 1}
	y0 := (float64(r0)*ts - cam.Y) * zoom
	y1 := (float64(r1)*ts - cam.Y) * zoom
	if y0 < cfg.ToolbarHeight {
		y0 = cfg.ToolbarHeight
	}
	h := y1 - y0
	if h > 0 {
		for col := c0; col <= c1; col++ {
			x := (float64(col)*ts - cam.X) * zoom
			ui.Rect(x, y0, 1, h, line)
		}
	}
	x0 := (float64(c0)*ts - cam.X) * zoom
	x1 := (float64(c1)*ts - cam.X) * zoom
	if x0 < 0 {
		x0 = 0
	}
	if x1 > playW {
		x1 = playW
	}
	w := x1 - x0
	if w > 0 {
		for row := r0; row <= r1; row++ {
			y := (float64(row)*ts - cam.Y) * zoom
			if y < cfg.ToolbarHeight {
				continue
			}
			ui.Rect(x0, y, w, 1, line)
		}
	}
}

func (s *GridState) renderPolylines(ui types.UIManager, cam *components.Camera) {
	if s.linePathFilter == nil {
		s.linePathFilter = ecs.NewFilter1[grid.LinePath](s.World())
	}
	col := types.Color{0.35, 0.65, 0.95, 1}
	s.linePathFilter.Each(func(_ ecs.Entity, lp *grid.LinePath) {
		drawThickPath(ui, cam, lp.Cells, col, 0.28)
	})
}

func (s *GridState) renderJunctions(ui types.UIManager, cam *components.Camera) {
	if s.junctionFilter == nil {
		s.junctionFilter = ecs.NewFilter1[grid.GridObject](s.World())
	}
	fill := types.Color{0.85, 0.85, 0.35, 1}
	ring := types.Color{0.2, 0.2, 0.15, 1}
	s.junctionFilter.Each(func(_ ecs.Entity, go_ *grid.GridObject) {
		if go_.Kind != grid.ToolJunction {
			return
		}
		drawCellCircle(ui, cam, go_.Cell, fill, ring, 0.35)
	})
}

func (s *GridState) renderDeviceGhostPorts(ui types.UIManager, cam *components.Camera, occupancy *grid.GridOccupancy) {
	if occupancy == nil {
		return
	}
	if s.gridObjectFilter == nil {
		s.gridObjectFilter = ecs.NewFilter1[grid.GridObject](s.World())
	}
	genGhost := types.Color{0.5, 0.75, 0.55, 0.55}
	genRing := types.Color{0.3, 0.5, 0.35, 0.7}
	houseGhost := types.Color{0.55, 0.6, 0.85, 0.55}
	houseRing := types.Color{0.35, 0.4, 0.6, 0.7}
	cfg := gameconfig.Global
	s.gridObjectFilter.Each(func(_ ecs.Entity, go_ *grid.GridObject) {
		var fill, ring types.Color
		switch go_.Kind {
		case grid.ToolGenerator:
			fill, ring = genGhost, genRing
		case grid.ToolHouse:
			fill, ring = houseGhost, houseRing
		default:
			return
		}
		for _, nb := range grid.CardinalNeighbours(go_.Cell) {
			if occupancy.Occupied(nb) {
				continue
			}
			if nb.Col < 0 || nb.Row < 0 || nb.Col >= cfg.GridCols || nb.Row >= cfg.GridRows {
				continue
			}
			drawCellCircle(ui, cam, nb, fill, ring, 0.28)
		}
	})
}

func drawThickPath(ui types.UIManager, cam *components.Camera, cells []grid.GridCoord, c types.Color, thicknessFrac float64) {
	if len(cells) < 2 {
		return
	}
	zoom := cam.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	ts := gameconfig.Global.TileSize
	thick := ts * zoom * thicknessFrac
	for i := 0; i < len(cells)-1; i++ {
		x0, y0 := cellCenterScreen(cam, cells[i])
		x1, y1 := cellCenterScreen(cam, cells[i+1])
		drawThickSegment(ui, x0, y0, x1, y1, thick, c)
	}
}

func cellCenterScreen(cam *components.Camera, cell grid.GridCoord) (x, y float64) {
	cx, cy, w, h := grid.CellScreenRect(cam, cell)
	return cx + w/2, cy + h/2
}

func drawThickSegment(ui types.UIManager, x0, y0, x1, y1, thick float64, c types.Color) {
	if x0 == x1 {
		// vertical
		y, h := y0, y1-y0
		if h < 0 {
			y, h = y1, -h
		}
		ui.Rect(x0-thick/2, y, thick, h, c)
		return
	}
	// horizontal (Manhattan paths are axis-aligned)
	x, w := x0, x1-x0
	if w < 0 {
		x, w = x1, -w
	}
	ui.Rect(x, y0-thick/2, w, thick, c)
}

func drawCellCircle(ui types.UIManager, cam *components.Camera, cell grid.GridCoord, fill, ring types.Color, radiusFrac float64) {
	x, y, w, h := grid.CellScreenRect(cam, cell)
	cx, cy := x+w/2, y+h/2
	r := w * radiusFrac
	// Approximate circle with nested squares (UI has no ellipse primitive).
	ui.Rect(cx-r, cy-r, 2*r, 2*r, ring)
	inner := r * 0.7
	ui.Rect(cx-inner, cy-inner, 2*inner, 2*inner, fill)
}

func (s *GridState) drawCellBorder(
	ui types.UIManager,
	cam *components.Camera,
	cell grid.GridCoord,
	c types.Color,
	thickness float64,
) {
	x, y, w, h := grid.CellScreenRect(cam, cell)
	ui.Rect(x, y, w, thickness, c)
	ui.Rect(x, y+h-thickness, w, thickness, c)
	ui.Rect(x, y, thickness, h, c)
	ui.Rect(x+w-thickness, y, thickness, h, c)
}

func (s *GridState) drawPlacementGhost(
	ui types.UIManager,
	cam *components.Camera,
	occupancy *grid.GridOccupancy,
	placement *grid.PlacementState,
) {
	if placement.Tool == grid.ToolLine {
		cells := []grid.GridCoord{placement.HoverCell}
		if placement.LinePending {
			cells = grid.ManhattanPath(placement.LineStart, placement.HoverCell)
		}
		ok := lineGhostOK(s.World(), occupancy, cells, placement)
		col := types.Color{0.2, 0.9, 0.35, 0.85}
		if !ok {
			col = types.Color{0.9, 0.25, 0.2, 0.85}
		}
		if len(cells) >= 2 {
			drawThickPath(ui, cam, cells, col, 0.22)
		} else {
			drawCellCircle(ui, cam, cells[0], col, types.Color{0, 0, 0, 0.5}, 0.3)
		}
		return
	}

	cell := placement.HoverCell
	occupied := occupancy != nil && occupancy.Occupied(cell)
	fill, fg := ghostColors(placement.Tool, occupied)
	x, y, w, h := grid.CellScreenRect(cam, cell)
	ui.Rect(x, y, w, h, fill)
	ui.TextColored(x+w*0.3, y+h*0.25, fg, placement.Tool.GhostLetter())
}

func lineGhostOK(w *ecs.World, occupancy *grid.GridOccupancy, cells []grid.GridCoord, placement *grid.PlacementState) bool {
	if len(cells) == 0 {
		return false
	}
	end := cells[len(cells)-1]
	if !lineEndOK(w, occupancy, end) {
		return false
	}
	if placement.LinePending && !lineEndOK(w, occupancy, placement.LineStart) {
		return false
	}
	for i, c := range cells {
		e, ok := occupancy.Cells[c]
		if !ok {
			continue
		}
		go_ := ecs.NewMap1[grid.GridObject](w).Get(e)
		if go_ == nil {
			return false
		}
		switch go_.Kind {
		case grid.ToolLine:
			continue
		case grid.ToolHouse, grid.ToolJunction:
			if i != 0 && i != len(cells)-1 {
				return false
			}
		case grid.ToolGenerator:
			return false
		default:
			return false
		}
	}
	return true
}

func lineEndOK(w *ecs.World, occupancy *grid.GridOccupancy, cell grid.GridCoord) bool {
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
	return true
}

func ghostColors(t grid.Tool, occupied bool) (fill, fg types.Color) {
	switch t {
	case grid.ToolDelete:
		if occupied {
			return types.Color{0.7, 0.1, 0.1, 0.45}, types.Color{1, 0.25, 0.25, 1}
		}
		return types.Color{0.3, 0.3, 0.3, 0.35}, types.Color{0.55, 0.55, 0.55, 1}
	default:
		if occupied {
			return types.Color{0.7, 0.15, 0.1, 0.4}, types.Color{1, 0.35, 0.3, 1}
		}
		return types.Color{0.15, 0.55, 0.25, 0.4}, types.Color{0.85, 1, 0.85, 1}
	}
}
