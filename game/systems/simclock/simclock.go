// Package simclock advances the world SimClock from real-time dt.
package simclock

import (
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/milo/pkg/ecs"
)

// SimClockSystem maps wall-clock dt into simulation milliseconds.
type SimClockSystem struct{}

// NewSimClockSystem returns a SimClockSystem.
func NewSimClockSystem() *SimClockSystem {
	return &SimClockSystem{}
}

// Update advances NowMs when Playing; otherwise clears DeltaMs.
func (s *SimClockSystem) Update(w *ecs.World, dt float64) {
	clock := ecs.GetResource[sim.SimClock](w)
	if clock == nil {
		return
	}
	if !clock.Playing || dt <= 0 {
		clock.DeltaMs = 0
		return
	}
	clock.DeltaMs = int64(float64(clock.SpeedMsPerRealSec()) * dt)
	clock.NowMs += clock.DeltaMs
}
