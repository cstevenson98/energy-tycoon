// Package loadtick implements LoadTickSystem, which periodically re-samples
// house demand and pushes the new P/Q into ElectricalNetwork bus specs so
// LoadflowSystem re-solves on the next dirty pass.
package loadtick

import (
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/gowasm-engine/pkg/ecs"
)

// DefaultIntervalMs is how often house loads are re-sampled in sim time.
// At the default clock speed (1 sim-hour / real-second) this is ~3 real seconds.
const DefaultIntervalMs = 3 * sim.MsPerHour

// LoadTickSystem fires on SimClock absolute time. When IntervalMs of sim time
// elapses, it assigns a new random P/Q to every HouseLoad entity that is
// linked into the network, then updates the corresponding BusSpec (which
// marks the network Dirty).
type LoadTickSystem struct {
	IntervalMs int64
	nextFireMs int64
	houses     *ecs.Filter2[grid.HouseLoad, network.NetworkLink]
}

// NewLoadTickSystem builds the system with DefaultIntervalMs.
// The first fire is after one full interval from the sim epoch.
func NewLoadTickSystem(w *ecs.World) *LoadTickSystem {
	return &LoadTickSystem{
		IntervalMs: DefaultIntervalMs,
		nextFireMs: sim.EpochMs + DefaultIntervalMs,
		houses:     ecs.NewFilter2[grid.HouseLoad, network.NetworkLink](w),
	}
}

// Update advances timers from SimClock; when nextFireMs is reached, all houses
// are re-sampled. Catches up multiple intervals if a fast frame jumps ahead.
func (s *LoadTickSystem) Update(w *ecs.World, _ float64) {
	if s.IntervalMs <= 0 {
		s.IntervalMs = DefaultIntervalMs
	}

	clock := ecs.GetResource[sim.SimClock](w)
	if clock == nil || clock.DeltaMs == 0 {
		return
	}

	net := ecs.GetResource[network.ElectricalNetwork](w)
	if net == nil {
		return
	}

	for clock.NowMs >= s.nextFireMs {
		s.houses.Each(func(_ ecs.Entity, hl *grid.HouseLoad, link *network.NetworkLink) {
			hl.PKw = grid.RandLoadKW()
			hl.QKw = grid.RandLoadKW()
			// Consumer kW → generator-convention watts (negative = load).
			net.SetBusSpec(link.BusID, network.PQSpec(-hl.PKw*1000, -hl.QKw*1000))
		})
		s.nextFireMs += s.IntervalMs
	}
}
