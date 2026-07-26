// Package loadflow implements LoadflowSystem: when the ElectricalNetwork
// resource is marked Dirty (topology or bus-spec change), it runs one AC
// power-flow solve. On frames where the circuit is unchanged it is a no-op.
package loadflow

import (
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/gowasm-engine/pkg/ecs"
	"github.com/cstevenson98/gowasm-engine/pkg/logger"
)

// LoadflowSystem owns network analysis. Placement (and any other mutator)
// only touches the ElectricalNetwork graph/specs and leaves Dirty set;
// this system is the sole caller of LoadflowSolver.Solve.
type LoadflowSystem struct {
	solver *network.LoadflowSolver
}

// NewLoadflowSystem builds the system with a default LoadflowSolver.
func NewLoadflowSystem(_ *ecs.World) *LoadflowSystem {
	return &LoadflowSystem{solver: network.NewLoadflowSolver()}
}

// Update re-solves the network if Dirty, then clears the flag. A failed
// solve still clears Dirty so we do not spam Newton-Raphson every frame
// for an unchanged (e.g. no-slack) circuit; the next mutation will mark
// Dirty again.
func (s *LoadflowSystem) Update(w *ecs.World, _ float64) {
	net := ecs.GetResource[network.ElectricalNetwork](w)
	if net == nil || !net.Dirty {
		return
	}

	if gameconfig.Global.DebugLoadflowLog {
		net.Log()
	}
	err := s.solver.Solve(net)
	if err != nil {
		// LastError is on net.State for ImGui; keep the log at Debug to avoid spam.
		logger.Logger.Debugf("grid-sim: loadflow failed: %v", err)
	}
	// Append history when the solve wrote results (success, or NR that ran
	// some iterations). Skip pure early-outs like "no slack bus".
	if err == nil || net.State.Converged || net.State.Iterations > 0 {
		network.RecordHistory(w, net)
	}
	if gameconfig.Global.DebugLoadflowLog {
		net.LogVoltages()
	}
	net.ClearDirty()
}
