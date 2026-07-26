package applianceload_test

import (
	"math"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/appliance"
	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/energy-tycoon/game/systems/applianceload"
	"github.com/cstevenson98/milo/pkg/ecs"
)

func TestApplianceLoadStepsAndUpdatesBus(t *testing.T) {
	w := ecs.NewWorld()
	net := network.NewElectricalNetwork()
	clock := &sim.SimClock{
		Playing:    true,
		NowMs:      sim.EpochMs,
		SpeedIndex: sim.DefaultSpeedIndex,
	}
	amb := appliance.NewAmbientTemp()
	amb.OutdoorC = 15
	ecs.SetResource(w, net)
	ecs.SetResource(w, clock)
	ecs.SetResource(w, amb)

	ctx := appliance.MakeContext(sim.EpochMs, 15, nil)
	ha := appliance.NewHouseAppliances(ctx, []appliance.Instance{
		{Kind: appliance.KindHVAC, RatedPKw: 2.5},
	})
	p0, q0 := appliance.AggregatePower(ha)
	indoor0 := appliance.IndoorC(&ha.Items[0])

	e := ecs.NewMap3[grid.HouseLoad, appliance.HouseAppliances, network.NetworkLink](w).NewEntity(
		&grid.HouseLoad{Source: grid.DemandAppliances, PKw: p0, QKw: q0},
		ha,
		&network.NetworkLink{},
	)
	bus, err := net.AddBus(e, network.BusLoad)
	if err != nil {
		t.Fatalf("AddBus: %v", err)
	}
	ecs.NewMap1[network.NetworkLink](w).Get(e).BusID = bus.ID
	net.SetBusSpec(bus.ID, network.PQSpec(-p0*1000, -q0*1000))
	net.ClearDirty()

	sys := applianceload.NewApplianceLoadSystem(w)
	// Four 15m ticks = 1 sim-hour of HVAC stepping.
	clock.NowMs = sim.EpochMs + sim.MsPerHour
	clock.DeltaMs = 1
	sys.Update(w, 0)

	hl := ecs.NewMap1[grid.HouseLoad](w).Get(e)
	ha2 := ecs.NewMap1[appliance.HouseAppliances](w).Get(e)
	wantP, wantQ := appliance.AggregatePower(ha2)
	if math.Abs(hl.PKw-wantP) > 1e-9 || math.Abs(hl.QKw-wantQ) > 1e-9 {
		t.Fatalf("HouseLoad P/Q = %v/%v, want %v/%v", hl.PKw, hl.QKw, wantP, wantQ)
	}
	if !net.Dirty {
		t.Fatal("expected Dirty after appliance tick")
	}
	indoor := appliance.IndoorC(&ha2.Items[0])
	if indoor <= indoor0 {
		t.Fatalf("indoor = %v, want > %v after heating from cold start", indoor, indoor0)
	}
}

func TestApplianceLoadPausedSkips(t *testing.T) {
	w := ecs.NewWorld()
	clock := &sim.SimClock{
		Playing: false,
		NowMs:   sim.EpochMs + 10*sim.MsPerHour,
		DeltaMs: 0,
	}
	ecs.SetResource(w, clock)
	ecs.SetResource(w, appliance.NewAmbientTemp())

	ctx := appliance.MakeContext(sim.EpochMs, 15, nil)
	ha := appliance.NewHouseAppliances(ctx, []appliance.Instance{
		{Kind: appliance.KindHVAC, RatedPKw: 2.5},
	})
	before := ha.Items[0].Mem[0]
	ecs.NewMap2[grid.HouseLoad, appliance.HouseAppliances](w).NewEntity(
		&grid.HouseLoad{Source: grid.DemandAppliances},
		ha,
	)

	applianceload.NewApplianceLoadSystem(w).Update(w, 0)
	if ha.Items[0].Mem[0] != before {
		t.Fatal("paused should not advance HVAC")
	}
}
