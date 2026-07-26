// Package pointer owns hover and ToolNone cell selection for the grid
// playfield. PlacementSystem handles toolbar tools and spawning.
package pointer

import (
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/milo/pkg/components"
	"github.com/cstevenson98/milo/pkg/ecs"
)

// PointerSystem updates hover every frame, clears the tool on C, and selects
// a cell when ToolNone and the player left-clicks the playfield.
type PointerSystem struct{}

// NewPointerSystem builds the system.
func NewPointerSystem(_ *ecs.World) *PointerSystem {
	return &PointerSystem{}
}

// Update refreshes hover, handles C-clear, and ToolNone selection clicks.
func (s *PointerSystem) Update(w *ecs.World, _ float64) {
	in := ecs.GetResource[components.Input](w)
	placement := ecs.GetResource[grid.PlacementState](w)
	cam := ecs.GetResource[components.Camera](w)
	if in == nil || placement == nil || cam == nil {
		return
	}

	mouse := in.State.Mouse
	bounds := ecs.GetResource[components.ScreenBounds](w)
	updateHover(placement, cam, bounds, mouse.X, mouse.Y)

	if in.State.CPressed && !in.State.CPressedLastFrame {
		placement.Tool = grid.ToolNone
		placement.LinePending = false
	}

	if !mouse.Left.Pressed || mouse.Left.PressedLastFrame {
		return
	}
	if bounds != nil && mouse.X >= gameconfig.Global.PlayfieldWidth(bounds.W) {
		return
	}
	if mouse.Y < gameconfig.Global.ToolbarHeight {
		return // toolbar belongs to PlacementSystem
	}
	if placement.Tool != grid.ToolNone {
		return
	}
	if placement.HoverValid {
		placement.HasSelection = true
		placement.SelectedCell = placement.HoverCell
	}
}

func updateHover(placement *grid.PlacementState, cam *components.Camera, bounds *components.ScreenBounds, x, y float64) {
	placement.HoverValid = false
	if bounds != nil && x >= gameconfig.Global.PlayfieldWidth(bounds.W) {
		return
	}
	if y < gameconfig.Global.ToolbarHeight {
		return
	}
	cell, ok := grid.ScreenToCell(cam, x, y)
	if !ok {
		return
	}
	placement.HoverCell = cell
	placement.HoverValid = true
}
