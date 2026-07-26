package grid

import (
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/milo/pkg/components"
)

// ScreenToCell converts a screen-space point (virtual pixels) to a grid cell
// using the camera's world offset and zoom. Returns false when the point is
// outside the grid bounds (caller should still treat toolbar / side panel
// separately).
//
// screen = (world - cam) * zoom  ⇒  world = cam + screen/zoom
func ScreenToCell(cam *components.Camera, screenX, screenY float64) (GridCoord, bool) {
	if cam == nil {
		return GridCoord{}, false
	}
	ts := gameconfig.Global.TileSize
	zoom := cam.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	cell := GridCoord{
		Col: int((cam.X + screenX/zoom) / ts),
		Row: int((cam.Y + screenY/zoom) / ts),
	}
	cfg := gameconfig.Global
	if cell.Col < 0 || cell.Row < 0 || cell.Col >= cfg.GridCols || cell.Row >= cfg.GridRows {
		return cell, false
	}
	return cell, true
}

// CellScreenRect returns the screen-space rectangle (x, y, w, h) for cell
// under the current camera. Used by overlay borders and placement ghosts.
func CellScreenRect(cam *components.Camera, cell GridCoord) (x, y, w, h float64) {
	ts := gameconfig.Global.TileSize
	zoom := 1.0
	var camX, camY float64
	if cam != nil {
		camX, camY = cam.X, cam.Y
		if cam.Zoom > 0 {
			zoom = cam.Zoom
		}
	}
	x = (float64(cell.Col)*ts - camX) * zoom
	y = (float64(cell.Row)*ts - camY) * zoom
	w = ts * zoom
	h = ts * zoom
	return x, y, w, h
}
