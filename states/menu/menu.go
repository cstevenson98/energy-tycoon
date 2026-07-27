package menustate

import (
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/energy-tycoon/game/ui/menu"
	"github.com/cstevenson98/milo/pkg/prefab"
	"github.com/cstevenson98/milo/pkg/state"
	"github.com/cstevenson98/milo/pkg/types"
)

var mainMenuItems = []string{"New", "Load", "Settings", "Quit"}

const (
	mainMenuTitleScale = 3.0
	mainMenuTitleBlock = 48.0
)

// MenuState is the main menu: New / Load / Settings / Quit stubs drawn as
// clickable buttons over the same blank background as GridState.
type MenuState struct {
	*state.BaseState
	list *menu.List
}

// NewMenuState creates the main menu state.
func NewMenuState() *MenuState {
	return &MenuState{BaseState: state.NewBaseState("Menu")}
}

// Enter seeds a full-screen blank background matching the gameplay look.
func (s *MenuState) Enter(deps state.Deps) error {
	if err := s.BaseState.Enter(deps); err != nil {
		return err
	}

	cfg := gameconfig.Global
	prefab.NewBackground(
		s.World(),
		types.Vector2{},
		types.Vector2{X: s.ScreenWidth(), Y: s.ScreenHeight()},
		cfg.BlankTexture,
	)
	s.list = s.layoutList()
	return nil
}

func (s *MenuState) layoutList() *menu.List {
	startY := s.ScreenHeight()*0.35 - mainMenuTitleBlock/2
	itemY := startY + mainMenuTitleBlock + 24
	x := (s.ScreenWidth() - menu.ButtonW) / 2
	return menu.NewList(mainMenuItems, x, itemY)
}

// Update navigates the menu; Esc quits the game.
func (s *MenuState) Update(dt float64) {
	in := s.Input()
	if in.EscPressed && !in.EscPressedLastFrame {
		s.RequestQuit()
		s.BaseState.Update(dt)
		return
	}

	if s.list == nil {
		s.list = s.layoutList()
	}
	if idx, ok := s.list.Update(in); ok {
		s.activate(idx)
	}

	s.BaseState.Update(dt)
}

func (s *MenuState) activate(idx int) {
	switch mainMenuItems[idx] {
	case "New":
		_ = s.RequestState(types.GAMEPLAY)
	case "Quit":
		s.RequestQuit()
	case "Load", "Settings":
		// Stubs for now.
	}
}

// DrawOverlays draws the title and menu buttons via the shared menu widget.
func (s *MenuState) DrawOverlays() error {
	ui := s.UI()
	lh := ui.LineHeight()
	if s.list == nil {
		s.list = s.layoutList()
	}

	titleY := s.list.Y - mainMenuTitleBlock
	ui.TextCenteredScaled(titleY, mainMenuTitleScale, types.White, "Energy Tycoon")
	s.list.Draw(ui)
	ui.TextCentered(s.ScreenHeight()-lh*2, types.Gray, "Click or Enter to confirm  Esc: quit")

	return s.BaseState.DrawOverlays()
}
