package grid

import "github.com/cstevenson98/energy-tycoon/game/gameconfig"

// ToolbarButton is one clickable button in the top toolbar: which Tool it
// selects, its label, and its screen-space hit rect. Shared between
// PlacementSystem (hit-testing clicks) and GridState.DrawOverlays (drawing
// the bar), so the two never disagree about button positions.
type ToolbarButton struct {
	Tool  Tool
	Label string
	X, Y  float64
	W, H  float64
}

// Contains reports whether the screen-space point (x, y) falls inside the
// button's rect.
func (b ToolbarButton) Contains(x, y float64) bool {
	return x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H
}

// ToolbarButtons returns the toolbar's buttons in display order, laid out
// left to right using the sizing/spacing in gameconfig.Global.
func ToolbarButtons() []ToolbarButton {
	cfg := gameconfig.Global
	tools := []Tool{ToolGenerator, ToolLine, ToolHouse, ToolDelete}

	buttons := make([]ToolbarButton, len(tools))
	x := cfg.ButtonMarginX
	for i, t := range tools {
		buttons[i] = ToolbarButton{
			Tool:  t,
			Label: t.Label(),
			X:     x,
			Y:     cfg.ButtonMarginY,
			W:     cfg.ButtonWidth,
			H:     cfg.ButtonHeight,
		}
		x += cfg.ButtonWidth + cfg.ButtonGap
	}
	return buttons
}
