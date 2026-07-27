package grid

import (
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/milo/pkg/ecs"
)

// InspectorPanel tracks whether the ImGui network inspector is shown.
// When hidden, the playfield uses the full screen width.
type InspectorPanel struct {
	Visible bool
}

// NewInspectorPanel returns a hidden inspector panel resource.
func NewInspectorPanel() *InspectorPanel {
	return &InspectorPanel{Visible: false}
}

// PlayfieldWidth returns the clickable/visible playfield width for the current
// inspector visibility. Falls back to the configured split when no resource.
func PlayfieldWidth(w *ecs.World, screenW float64) float64 {
	if p := ecs.GetResource[InspectorPanel](w); p != nil && !p.Visible {
		return screenW
	}
	return gameconfig.Global.PlayfieldWidth(screenW)
}
