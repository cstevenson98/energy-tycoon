package loadtick_test

import (
	"math"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/energy-tycoon/game/systems/loadtick"
	"github.com/cstevenson98/milo/pkg/ecs"
)

func TestLoadTickEvaluatesProfileWhenDue(t *testing.T) {
	w := ecs.NewWorld()
	net := network.NewElectricalNetwork()
	clock := &sim.SimClock{
		Playing:    true,
		NowMs:      sim.EpochMs,
		SpeedIndex: sim.DefaultSpeedIndex,
	}
	ecs.SetResource(w, net)
	ecs.SetResource(w, clock)

	const peak = 5.0
	e := ecs.NewMap2[grid.HouseLoad, network.NetworkLink](w).NewEntity(
		&grid.HouseLoad{Source: grid.DemandProfile, Profile: grid.ProfileSummerResidential, PeakKW: peak, PKw: 0, QKw: 0},
		&network.NetworkLink{},
	)
	bus, err := net.AddBus(e, network.BusLoad)
	if err != nil {
		t.Fatalf("AddBus: %v", err)
	}
	ecs.NewMap1[network.NetworkLink](w).Get(e).BusID = bus.ID
	net.SetBusSpec(bus.ID, network.PQSpec(0, 0))
	net.ClearDirty()

	sys := loadtick.NewLoadTickSystem(w)
	// 19:00 — summer residential peak knot; also past the first 3h fire.
	clock.NowMs = sim.EpochMs + 19*sim.MsPerHour
	clock.DeltaMs = 1
	sys.Update(w, 0)

	wantP, wantQ := grid.DemandKW(grid.ProfileSummerResidential, peak, sim.DayFraction(clock.NowMs))
	hl := ecs.NewMap1[grid.HouseLoad](w).Get(e)
	if math.Abs(hl.PKw-wantP) > 1e-9 || math.Abs(hl.QKw-wantQ) > 1e-9 {
		t.Fatalf("got P=%v Q=%v, want P=%v Q=%v", hl.PKw, hl.QKw, wantP, wantQ)
	}
	if !net.Dirty {
		t.Fatal("expected network Dirty after load tick")
	}
}

func TestLoadTickPausedSkips(t *testing.T) {
	w := ecs.NewWorld()
	net := network.NewElectricalNetwork()
	clock := &sim.SimClock{
		Playing:    false,
		NowMs:      sim.EpochMs + loadtick.DefaultIntervalMs*10,
		DeltaMs:    0,
		SpeedIndex: sim.DefaultSpeedIndex,
	}
	ecs.SetResource(w, net)
	ecs.SetResource(w, clock)

	e := ecs.NewMap2[grid.HouseLoad, network.NetworkLink](w).NewEntity(
		&grid.HouseLoad{Source: grid.DemandProfile, Profile: grid.ProfileSummerResidential, PeakKW: 5, PKw: 2.0, QKw: 2.0},
		&network.NetworkLink{},
	)
	bus, err := net.AddBus(e, network.BusLoad)
	if err != nil {
		t.Fatalf("AddBus: %v", err)
	}
	ecs.NewMap1[network.NetworkLink](w).Get(e).BusID = bus.ID
	net.SetBusSpec(bus.ID, network.PQSpec(-2000, -2000))
	net.ClearDirty()

	loadtick.NewLoadTickSystem(w).Update(w, 0)

	hl := ecs.NewMap1[grid.HouseLoad](w).Get(e)
	if hl.PKw != 2.0 || hl.QKw != 2.0 {
		t.Fatalf("paused should not resample, got P=%v Q=%v", hl.PKw, hl.QKw)
	}
	if net.Dirty {
		t.Fatal("paused should not mark Dirty")
	}
}

func TestLoadTickSkipsApplianceHouses(t *testing.T) {
	w := ecs.NewWorld()
	net := network.NewElectricalNetwork()
	clock := &sim.SimClock{
		Playing:    true,
		NowMs:      sim.EpochMs + loadtick.DefaultIntervalMs,
		DeltaMs:    1,
		SpeedIndex: sim.DefaultSpeedIndex,
	}
	ecs.SetResource(w, net)
	ecs.SetResource(w, clock)

	e := ecs.NewMap2[grid.HouseLoad, network.NetworkLink](w).NewEntity(
		&grid.HouseLoad{Source: grid.DemandAppliances, PKw: 1.5, QKw: 0.5},
		&network.NetworkLink{},
	)
	bus, err := net.AddBus(e, network.BusLoad)
	if err != nil {
		t.Fatalf("AddBus: %v", err)
	}
	ecs.NewMap1[network.NetworkLink](w).Get(e).BusID = bus.ID
	net.ClearDirty()

	loadtick.NewLoadTickSystem(w).Update(w, 0)

	hl := ecs.NewMap1[grid.HouseLoad](w).Get(e)
	if hl.PKw != 1.5 || hl.QKw != 0.5 {
		t.Fatalf("appliance house should be skipped, got P=%v Q=%v", hl.PKw, hl.QKw)
	}
	if net.Dirty {
		t.Fatal("appliance house should not mark Dirty via loadtick")
	}
}

func TestLoadTickNotYetDue(t *testing.T) {
	w := ecs.NewWorld()
	net := network.NewElectricalNetwork()
	clock := &sim.SimClock{
		Playing:    true,
		NowMs:      sim.EpochMs + loadtick.DefaultIntervalMs - 1,
		DeltaMs:    1,
		SpeedIndex: sim.DefaultSpeedIndex,
	}
	ecs.SetResource(w, net)
	ecs.SetResource(w, clock)

	e := ecs.NewMap2[grid.HouseLoad, network.NetworkLink](w).NewEntity(
		&grid.HouseLoad{Source: grid.DemandProfile, Profile: grid.ProfileSummerResidential, PeakKW: 5, PKw: 2.0, QKw: 2.0},
		&network.NetworkLink{},
	)
	bus, err := net.AddBus(e, network.BusLoad)
	if err != nil {
		t.Fatalf("AddBus: %v", err)
	}
	ecs.NewMap1[network.NetworkLink](w).Get(e).BusID = bus.ID
	net.ClearDirty()

	loadtick.NewLoadTickSystem(w).Update(w, 0)

	hl := ecs.NewMap1[grid.HouseLoad](w).Get(e)
	if hl.PKw != 2.0 {
		t.Fatalf("too early to resample, got P=%v", hl.PKw)
	}
	if net.Dirty {
		t.Fatal("should not mark Dirty before interval")
	}
}
