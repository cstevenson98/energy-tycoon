// Package menu provides a reusable vertical button list for screen-space
// overlays (main menu, pause menu, etc.). Font glyphs are white-only, so the
// selected button uses a yellow outline over a dark fill.
package menu

import "github.com/cstevenson98/milo/pkg/types"

// Default button sizing shared by menus across the game.
const (
	ButtonW   = 160.0
	ButtonH   = 24.0
	ButtonGap = 10.0
)

// ButtonFill is the dark button face; Outline is drawn around the selection.
var (
	ButtonFill = types.Color{0.3, 0.3, 0.34, 1}
	PanelFill  = types.Color{0.12, 0.12, 0.15, 1}
	Dimmer     = types.Color{0, 0, 0, 0.55}
)

// Button is one clickable entry and its screen-space hit rect.
type Button struct {
	Label  string
	X, Y   float64
	W, H   float64
	Index  int
}

// Contains reports whether (x, y) lies inside the button.
func (b Button) Contains(x, y float64) bool {
	return x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H
}

// List is a vertical stack of labeled buttons with keyboard/mouse selection.
type List struct {
	Items    []string
	Selected int
	X, Y     float64 // top-left of the first button
	ButtonW  float64
	ButtonH  float64
	Gap      float64
}

// NewList builds a list with default button sizing at (x, y).
func NewList(items []string, x, y float64) *List {
	return &List{
		Items:   items,
		X:       x,
		Y:       y,
		ButtonW: ButtonW,
		ButtonH: ButtonH,
		Gap:     ButtonGap,
	}
}

// Height returns the total vertical span of the button stack.
func (l *List) Height() float64 {
	n := len(l.Items)
	if n == 0 {
		return 0
	}
	return float64(n)*l.ButtonH + float64(n-1)*l.Gap
}

// Buttons returns hit rects in display order.
func (l *List) Buttons() []Button {
	out := make([]Button, len(l.Items))
	for i, label := range l.Items {
		out[i] = Button{
			Label: label,
			X:     l.X,
			Y:     l.Y + float64(i)*(l.ButtonH+l.Gap),
			W:     l.ButtonW,
			H:     l.ButtonH,
			Index: i,
		}
	}
	return out
}

// Update handles arrow navigation, hover, and click/Enter activation.
// ok is true when a button was activated; index is that button's index.
func (l *List) Update(in types.InputState) (index int, ok bool) {
	n := len(l.Items)
	if n == 0 {
		return 0, false
	}
	if l.Selected < 0 {
		l.Selected = 0
	}
	if l.Selected >= n {
		l.Selected = n - 1
	}

	if in.UpPressed && !in.UpPressedLastFrame {
		l.Selected = (l.Selected - 1 + n) % n
	}
	if in.DownPressed && !in.DownPressedLastFrame {
		l.Selected = (l.Selected + 1) % n
	}

	for _, b := range l.Buttons() {
		if !b.Contains(in.Mouse.X, in.Mouse.Y) {
			continue
		}
		l.Selected = b.Index
		if in.Mouse.Left.Pressed && !in.Mouse.Left.PressedLastFrame {
			return b.Index, true
		}
		break
	}

	if in.EnterPressed && !in.EnterPressedLastFrame {
		return l.Selected, true
	}
	return 0, false
}

// Draw renders the button stack with the shared selected-outline style.
func (l *List) Draw(ui types.UIManager) {
	lh := ui.LineHeight()
	for _, b := range l.Buttons() {
		if b.Index == l.Selected {
			ui.Rect(b.X-2, b.Y-2, b.W+4, b.H+4, types.Yellow)
		}
		ui.Rect(b.X, b.Y, b.W, b.H, ButtonFill)
		labelW := ui.Measure(b.Label)
		ui.TextColored(b.X+(b.W-labelW)/2, b.Y+(b.H-lh)/2, types.White, b.Label)
	}
}

// DrawDimmer paints a semi-transparent full-screen filter.
func DrawDimmer(ui types.UIManager, screenW, screenH float64) {
	ui.Rect(0, 0, screenW, screenH, Dimmer)
}

// DrawPanel paints a solid panel behind a centered menu.
func DrawPanel(ui types.UIManager, x, y, w, h float64) {
	ui.Rect(x, y, w, h, PanelFill)
}
