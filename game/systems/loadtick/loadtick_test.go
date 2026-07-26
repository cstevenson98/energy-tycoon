package loadtick_test

import (
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/energy-tycoon/game/systems/loadtick"
	"github.com/cstevenson98/milo/pkg/ecs"
)

func TestLoadTickResamplesWhenDue(t *testing.T) {
	w := ecs.NewWorld()
	net := network.NewElectricalNetwork()
	clock := &sim.SimClock{
		Playing:    true,
		NowMs:      sim.EpochMs,
		SpeedIndex: sim.DefaultSpeedIndex,
	}
	ecs.SetResource(w, net)
	ecs.SetResource(w, clock)

	e := ecs.NewMap2[grid.HouseLoad, network.NetworkLink](w).NewEntity(
		&grid.HouseLoad{PKw: 0, QKw: 0},
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
	clock.NowMs = sim.EpochMs + loadtick.DefaultIntervalMs
	clock.DeltaMs = 1
	sys.Update(w, 0)

	hl := ecs.NewMap1[grid.HouseLoad](w).Get(e)
	if hl.PKw < grid.HouseLoadMinKW || hl.PKw > grid.HouseLoadMaxKW {
		t.Fatalf("PKw = %v, want in [%v,%v]", hl.PKw, grid.HouseLoadMinKW, grid.HouseLoadMaxKW)
	}
	if hl.QKw < grid.HouseLoadMinKW || hl.QKw > grid.HouseLoadMaxKW {
		t.Fatalf("QKw = %v, want in [%v,%v]", hl.QKw, grid.HouseLoadMinKW, grid.HouseLoadMaxKW)
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
		&grid.HouseLoad{PKw: 2.0, QKw: 2.0},
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
		&grid.HouseLoad{PKw: 2.0, QKw: 2.0},
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
