// Package applianceload advances appliances on a fixed sim-time tick and
// writes aggregated demand into HouseLoad / bus specs.
package applianceload

import (
	"math/rand"

	"github.com/cstevenson98/energy-tycoon/game/components/appliance"
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/milo/pkg/ecs"
)

// DefaultIntervalMs is how often appliances are stepped in sim time.
// At default clock speed (1 sim-hour / real-second) this is ~0.25 real seconds.
const DefaultIntervalMs = 15 * sim.MsPerMinute

// ApplianceLoadSystem steps every appliance house on a fixed interval.
type ApplianceLoadSystem struct {
	IntervalMs int64
	nextFireMs int64
	houses     *ecs.Filter2[grid.HouseLoad, appliance.HouseAppliances]
	links      *ecs.Map1[network.NetworkLink]
	rng        *rand.Rand
}

// NewApplianceLoadSystem builds the system with DefaultIntervalMs.
func NewApplianceLoadSystem(w *ecs.World) *ApplianceLoadSystem {
	return &ApplianceLoadSystem{
		IntervalMs: DefaultIntervalMs,
		nextFireMs: sim.EpochMs + DefaultIntervalMs,
		houses:     ecs.NewFilter2[grid.HouseLoad, appliance.HouseAppliances](w),
		links:      ecs.NewMap1[network.NetworkLink](w),
		rng:        rand.New(rand.NewSource(1)),
	}
}

// Update advances ambient temperature and steps appliances when the interval elapses.
func (s *ApplianceLoadSystem) Update(w *ecs.World, _ float64) {
	if s.IntervalMs <= 0 {
		s.IntervalMs = DefaultIntervalMs
	}

	clock := ecs.GetResource[sim.SimClock](w)
	if clock == nil || clock.DeltaMs == 0 {
		return
	}

	ambient := ecs.GetResource[appliance.AmbientTemp](w)
	net := ecs.GetResource[network.ElectricalNetwork](w)

	for clock.NowMs >= s.nextFireMs {
		if ambient != nil {
			ambient.Advance(clock.NowMs, s.rng)
		}
		outdoor := 20.0
		if ambient != nil {
			outdoor = ambient.OutdoorC
		}
		ctx := appliance.MakeContext(clock.NowMs, outdoor, s.rng)

		s.houses.Each(func(e ecs.Entity, hl *grid.HouseLoad, ha *appliance.HouseAppliances) {
			if hl.Source != grid.DemandAppliances {
				return
			}
			appliance.StepAll(ctx, ha, s.IntervalMs)
			hl.PKw, hl.QKw = appliance.AggregatePower(ha)
			if net != nil {
				if link := s.links.Get(e); link != nil {
					net.SetBusSpec(link.BusID, network.PQSpec(-hl.PKw*1000, -hl.QKw*1000))
				}
			}
		})
		s.nextFireMs += s.IntervalMs
	}
}
