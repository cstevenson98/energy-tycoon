// Package camera implements CameraScrollSystem, a free-scrolling / zoomable
// camera driven by keyboard/gamepad, middle-mouse drag, and mouse-wheel zoom,
// clamped to the grid's world bounds with chrome overscroll.
package camera

import (
	"math"

	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/milo/pkg/components"
	"github.com/cstevenson98/milo/pkg/ecs"
)

const (
	minZoom = 0.35
	maxZoom = 3.0
	// zoomStep is the multiplicative factor per unit of WheelY (scroll up → zoom in).
	zoomStep = 1.12
)

// CameraScrollSystem moves the World's Camera resource continuously while an
// arrow key (or WASD, or gamepad d-pad/stick) is held, or while the middle
// mouse button is held and dragged. The mouse wheel zooms toward the cursor
// (ignored over the ImGui side panel). Clamping allows a little overscroll
// past the map edges so rows under the toolbar (and the far edge against the
// side panel) can be brought fully into the clear playfield.
type CameraScrollSystem struct {
	speed float64

	// Middle-mouse drag state (screen-space cursor on the previous frame).
	dragging   bool
	lastMouseX float64
	lastMouseY float64
}

// NewCameraScrollSystem builds the system with the given keyboard scroll speed
// (world pixels per second at zoom 1). Middle-mouse drag pans 1:1 with cursor
// motion in screen space (converted through zoom).
func NewCameraScrollSystem(speed float64) *CameraScrollSystem {
	return &CameraScrollSystem{speed: speed}
}

// Update advances the camera and clamps it with chrome-aware overscroll.
func (s *CameraScrollSystem) Update(w *ecs.World, dt float64) {
	cam := ecs.GetResource[components.Camera](w)
	bounds := ecs.GetResource[components.ScreenBounds](w)
	if cam == nil || bounds == nil {
		return
	}
	if cam.Zoom <= 0 {
		cam.Zoom = 1
	}
	// Keep zoom on a discrete ladder so on-screen tile size is an integer
	// number of virtual pixels (nearest-neighbor looks clean; no hairline gaps).
	cam.Zoom = quantizeZoom(cam.Zoom, gameconfig.Global.TileSize)

	if in := ecs.GetResource[components.Input](w); in != nil {
		st := in.State
		if st.MoveLeft {
			cam.X -= s.speed * dt
		}
		if st.MoveRight {
			cam.X += s.speed * dt
		}
		if st.MoveUp {
			cam.Y -= s.speed * dt
		}
		if st.MoveDown {
			cam.Y += s.speed * dt
		}
		s.applyMiddleMousePan(cam, st.Mouse.X, st.Mouse.Y, st.Mouse.Middle.Pressed)
		s.applyWheelZoom(w, cam, bounds.W, st.Mouse.X, st.Mouse.Y, st.Mouse.WheelY)
	} else {
		s.dragging = false
	}

	minX, maxX, minY, maxY := scrollLimits(w, bounds.W, bounds.H, cam.Zoom)
	cam.X = clamp(cam.X, minX, maxX)
	cam.Y = clamp(cam.Y, minY, maxY)
	snapCameraToPixelGrid(cam)
}

// applyMiddleMousePan pans the camera so the world tracks the cursor while
// the middle button is held (drag right → look left). Delta is converted from
// screen pixels to world units via 1/zoom.
func (s *CameraScrollSystem) applyMiddleMousePan(cam *components.Camera, mx, my float64, middleHeld bool) {
	if !middleHeld {
		s.dragging = false
		return
	}
	if !s.dragging {
		s.dragging = true
		s.lastMouseX = mx
		s.lastMouseY = my
		return
	}
	z := cam.Zoom
	if z <= 0 {
		z = 1
	}
	cam.X -= (mx - s.lastMouseX) / z
	cam.Y -= (my - s.lastMouseY) / z
	s.lastMouseX = mx
	s.lastMouseY = my
}

// applyWheelZoom zooms toward the cursor so the world point under the mouse
// stays fixed. Scroll over the ImGui panel is ignored.
func (s *CameraScrollSystem) applyWheelZoom(w *ecs.World, cam *components.Camera, screenW, mx, my, wheelY float64) {
	if wheelY == 0 {
		return
	}
	if mx >= grid.PlayfieldWidth(w, screenW) {
		return
	}

	oldZ := cam.Zoom
	if oldZ <= 0 {
		oldZ = 1
	}
	newZ := quantizeZoom(oldZ*math.Pow(zoomStep, wheelY), gameconfig.Global.TileSize)
	if newZ == oldZ {
		return
	}

	// world = cam + screen/zoom  ⇒  keep world under cursor stable across zoom.
	worldX := cam.X + mx/oldZ
	worldY := cam.Y + my/oldZ
	cam.Zoom = newZ
	cam.X = worldX - mx/newZ
	cam.Y = worldY - my/newZ
}

// quantizeZoom snaps z so tileSize*z is an integer ≥ 1 virtual pixel, clamped
// to [minZoom, maxZoom]. Fractional on-screen tile sizes are the main source
// of zoom-out shimmer with nearest-neighbor filtering.
func quantizeZoom(z, tileSize float64) float64 {
	if tileSize <= 0 {
		tileSize = 32
	}
	if z <= 0 {
		z = 1
	}
	minPx := math.Max(1, math.Round(tileSize*minZoom))
	maxPx := math.Max(minPx, math.Round(tileSize*maxZoom))
	px := math.Round(tileSize * z)
	if px < minPx {
		px = minPx
	}
	if px > maxPx {
		px = maxPx
	}
	return px / tileSize
}

// snapCameraToPixelGrid rounds the camera so world origins land on virtual
// pixel centres after *zoom (pairs with quantizeZoom for crisp tiles).
func snapCameraToPixelGrid(cam *components.Camera) {
	z := cam.Zoom
	if z <= 0 {
		z = 1
	}
	cam.X = math.Round(cam.X*z) / z
	cam.Y = math.Round(cam.Y*z) / z
}

// scrollLimits returns inclusive camera offset bounds for the playfield at
// the given zoom. View size in world units shrinks as zoom increases.
//
// Negative minY (-ToolbarHeight/zoom in world space...): the toolbar is a
// screen-space chrome band; overscroll of ToolbarHeight screen pixels equals
// ToolbarHeight/zoom world units.
func scrollLimits(w *ecs.World, screenW, screenH, zoom float64) (minX, maxX, minY, maxY float64) {
	if zoom <= 0 {
		zoom = 1
	}
	cfg := gameconfig.Global
	viewW := grid.PlayfieldWidth(w, screenW) / zoom
	viewH := screenH / zoom

	minX = 0
	maxX = cfg.WorldWidth() - viewW
	if maxX < minX {
		maxX = minX
	}

	minY = -cfg.ToolbarHeight / zoom
	maxY = cfg.WorldHeight() - viewH
	if maxY < minY {
		// Map shorter than the view: pin so the top row clears the toolbar.
		maxY = minY
	}
	return minX, maxX, minY, maxY
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
