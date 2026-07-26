package simclock_test

import (
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/energy-tycoon/game/systems/simclock"
	"github.com/cstevenson98/milo/pkg/ecs"
)

func TestAdvancesAtDefaultSpeed(t *testing.T) {
	w := ecs.NewWorld()
	clock := sim.NewSimClock()
	ecs.SetResource(w, clock)

	simclock.NewSimClockSystem().Update(w, 1.0)

	if clock.DeltaMs != sim.MsPerHour {
		t.Fatalf("DeltaMs = %d, want %d", clock.DeltaMs, sim.MsPerHour)
	}
	want := sim.EpochMs + sim.MsPerHour
	if clock.NowMs != want {
		t.Fatalf("NowMs = %d, want %d", clock.NowMs, want)
	}
}

func TestPausedDoesNotAdvance(t *testing.T) {
	w := ecs.NewWorld()
	clock := sim.NewSimClock()
	clock.Playing = false
	clock.NowMs = sim.EpochMs + 42
	ecs.SetResource(w, clock)

	simclock.NewSimClockSystem().Update(w, 1.0)

	if clock.DeltaMs != 0 {
		t.Fatalf("DeltaMs = %d, want 0", clock.DeltaMs)
	}
	if clock.NowMs != sim.EpochMs+42 {
		t.Fatalf("NowMs = %d, want %d", clock.NowMs, sim.EpochMs+42)
	}
}

func TestWeekSpeed(t *testing.T) {
	w := ecs.NewWorld()
	clock := sim.NewSimClock()
	clock.SetSpeedIndex(4)
	ecs.SetResource(w, clock)

	simclock.NewSimClockSystem().Update(w, 1.0)

	if clock.DeltaMs != sim.MsPerWeek {
		t.Fatalf("DeltaMs = %d, want %d", clock.DeltaMs, sim.MsPerWeek)
	}
}
