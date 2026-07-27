package gridstate

import (
	"github.com/cstevenson98/energy-tycoon/game/components/appliance"
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/energy-tycoon/game/systems/applianceload"
	"github.com/cstevenson98/energy-tycoon/game/systems/camera"
	"github.com/cstevenson98/energy-tycoon/game/systems/loadflow"
	"github.com/cstevenson98/energy-tycoon/game/systems/loadtick"
	"github.com/cstevenson98/energy-tycoon/game/systems/placement"
	"github.com/cstevenson98/energy-tycoon/game/systems/pointer"
	"github.com/cstevenson98/energy-tycoon/game/systems/simclock"
	"github.com/cstevenson98/energy-tycoon/game/ui/menu"
	"github.com/cstevenson98/milo/pkg/components"
	"github.com/cstevenson98/milo/pkg/ecs"
	"github.com/cstevenson98/milo/pkg/imgui"
	"github.com/cstevenson98/milo/pkg/prefab"
	"github.com/cstevenson98/milo/pkg/state"
	"github.com/cstevenson98/milo/pkg/types"
)

// GridState is the gameplay state: a scrollable grid the player populates
// with generators, houses and lines via a clickable toolbar.
type GridState struct {
	*state.BaseState
	linePathFilter        *ecs.Filter1[grid.LinePath]  // thick polylines
	junctionFilter        *ecs.Filter1[grid.GridObject] // junction circles
	gridObjectFilter      *ecs.Filter1[grid.GridObject] // gen ghost ports
	voltageProfileYPadIdx int                          // index into voltageProfileYPadOptions

	paused    bool
	pauseList *menu.List
}

// NewGridState creates the grid state.
func NewGridState() *GridState {
	return &GridState{
		BaseState:             state.NewBaseState("Grid"),
		voltageProfileYPadIdx: 0, // ±1%
	}
}

// Enter seeds resources and registers systems. Empty cells are drawn
// procedurally (no blank ECS entities).
func (s *GridState) Enter(deps state.Deps) error {
	if err := s.BaseState.Enter(deps); err != nil {
		return err
	}

	s.paused = false
	s.pauseList = nil

	ecs.SetResource(s.World(), &grid.PlacementState{Tool: grid.ToolNone})
	ecs.SetResource(s.World(), grid.NewGridOccupancy())
	ecs.SetResource(s.World(), network.NewElectricalNetwork())
	ecs.SetResource(s.World(), sim.NewSimClock())
	ecs.SetResource(s.World(), appliance.NewAmbientTemp())
	ecs.SetResource(s.World(), grid.NewInspectorPanel())

	cfg := gameconfig.Global
	if cam := ecs.GetResource[components.Camera](s.World()); cam != nil {
		cam.Y = -cfg.ToolbarHeight
	}

	// One background quad under entities (overlays must not paint opaque fills
	// over the playfield — they run after the world pass).
	prefab.NewBackground(
		s.World(),
		types.Vector2{},
		types.Vector2{X: cfg.WorldWidth(), Y: cfg.WorldHeight()},
		cfg.BlankTexture,
	)

	// Pointer (hover/select) before placement; simclock advances sim time;
	// appliance-load / load-tick mutate P/Q; LoadflowSystem re-solves only
	// when Dirty; camera scrolls last.
	s.Schedule().
		Add(pointer.NewPointerSystem(s.World())).
		Add(placement.NewPlacementSystem(s.World())).
		Add(simclock.NewSimClockSystem()).
		Add(applianceload.NewApplianceLoadSystem(s.World())).
		Add(loadtick.NewLoadTickSystem(s.World())).
		Add(loadflow.NewLoadflowSystem(s.World())).
		Add(camera.NewCameraScrollSystem(cfg.CameraSpeed))

	return nil
}

// DrawOverlays draws the toolbar, procedural grid chrome, pause menu, and debug console.
func (s *GridState) DrawOverlays() error {
	s.renderToolbar()
	s.renderGridChrome()
	if s.paused {
		s.renderPauseOverlay()
	}
	return s.BaseState.DrawOverlays()
}

// RenderImGui draws the right-half network inspector panel when visible.
func (s *GridState) RenderImGui(ctx *imgui.Context) {
	panel := ecs.GetResource[grid.InspectorPanel](s.World())
	if panel != nil && !panel.Visible {
		return
	}

	cfg := gameconfig.Global
	screenW := s.ScreenWidth()
	screenH := s.ScreenHeight()
	panelW := cfg.SidePanelWidth(screenW)
	panelX := screenW - panelW

	ctx.Panel("Network", panelX, 0, panelW, screenH, func(w *imgui.WindowBuilder) {
		net := ecs.GetResource[network.ElectricalNetwork](s.World())
		if net == nil {
			w.Text("No electrical network resource")
			return
		}
		s.renderNetworkPanel(w, net)
	})
}
