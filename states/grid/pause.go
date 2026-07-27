package gridstate

import (
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/ui/menu"
	"github.com/cstevenson98/milo/pkg/ecs"
	"github.com/cstevenson98/milo/pkg/types"
)

var pauseMenuItems = []string{"Resume", "Save", "Load", "Quit"}

const (
	pauseTitleScale = 2.0
	pauseTitleBlock = 28.0
	pausePanelPad   = 16.0
)

// Update runs gameplay systems, or the pause overlay when open.
func (s *GridState) Update(dt float64) {
	in := s.Input()
	if in.EscPressed && !in.EscPressedLastFrame {
		s.paused = !s.paused
		if s.paused {
			s.pauseList = s.layoutPauseList()
		}
	}

	if in.AtTyped {
		if panel := ecs.GetResource[grid.InspectorPanel](s.World()); panel != nil {
			panel.Visible = !panel.Visible
		}
	}

	if s.paused {
		s.updatePauseMenu(in)
		return
	}

	s.BaseState.Update(dt)
}

func (s *GridState) updatePauseMenu(in types.InputState) {
	if s.pauseList == nil {
		s.pauseList = s.layoutPauseList()
	}
	idx, ok := s.pauseList.Update(in)
	if !ok {
		return
	}
	switch pauseMenuItems[idx] {
	case "Resume":
		s.paused = false
	case "Quit":
		s.paused = false
		_ = s.RequestState(types.MENU)
	case "Save", "Load":
		// Stubs for now.
	}
}

func (s *GridState) layoutPauseList() *menu.List {
	sw, sh := s.ScreenWidth(), s.ScreenHeight()
	panelW := menu.ButtonW + pausePanelPad*2
	listH := menu.NewList(pauseMenuItems, 0, 0).Height()
	panelH := pausePanelPad + pauseTitleBlock + listH + pausePanelPad
	panelX := (sw - panelW) / 2
	panelY := (sh - panelH) / 2
	listX := panelX + pausePanelPad
	listY := panelY + pausePanelPad + pauseTitleBlock
	list := menu.NewList(pauseMenuItems, listX, listY)
	if s.pauseList != nil {
		list.Selected = s.pauseList.Selected
	}
	return list
}

func (s *GridState) pausePanelRect() (x, y, w, h float64) {
	list := s.pauseList
	if list == nil {
		list = s.layoutPauseList()
	}
	w = menu.ButtonW + pausePanelPad*2
	h = pausePanelPad + pauseTitleBlock + list.Height() + pausePanelPad
	x = list.X - pausePanelPad
	y = list.Y - pauseTitleBlock - pausePanelPad
	return x, y, w, h
}

func (s *GridState) renderPauseOverlay() {
	ui := s.UI()
	sw, sh := s.ScreenWidth(), s.ScreenHeight()
	menu.DrawDimmer(ui, sw, sh)

	if s.pauseList == nil {
		s.pauseList = s.layoutPauseList()
	}
	px, py, pw, ph := s.pausePanelRect()
	menu.DrawPanel(ui, px, py, pw, ph)

	titleY := s.pauseList.Y - pauseTitleBlock + 4
	ui.TextCenteredScaled(titleY, pauseTitleScale, types.White, "Paused")
	s.pauseList.Draw(ui)
}
