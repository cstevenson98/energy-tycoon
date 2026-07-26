package appliance_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/appliance"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
)

func TestHVACTurnsOnWhenAwayFromSetpoint(t *testing.T) {
	b := appliance.Lookup(appliance.KindHVAC)
	if b == nil {
		t.Fatal("hvac not registered")
	}
	inst := &appliance.Instance{Kind: appliance.KindHVAC, RatedPKw: 2.5, Mem: [8]float64{0, 20}}
	ctx := appliance.MakeContext(sim.EpochMs, 15, nil)
	b.Init(ctx, inst)
	if !inst.On {
		t.Fatal("expected HVAC on when indoor starts at 15 °C (setpoint 20)")
	}
	// At T=Tout=15: leak 0, heat +2.5 °C/h → 17.5
	b.Step(ctx, inst, sim.MsPerHour)
	if math.Abs(appliance.IndoorC(inst)-17.5) > 1e-9 {
		t.Fatalf("indoor after 1h = %v, want 17.5", appliance.IndoorC(inst))
	}
	p, _ := b.PowerKW(inst)
	if p != 2.5 {
		t.Fatalf("PowerKW = %v, want 2.5", p)
	}
}

func TestHVACOffLeaksTowardOutdoor(t *testing.T) {
	b := appliance.Lookup(appliance.KindHVAC)
	inst := &appliance.Instance{Kind: appliance.KindHVAC, RatedPKw: 2.5, Mem: [8]float64{0, 20}}
	ctx := appliance.MakeContext(sim.EpochMs, 20, nil)
	b.Init(ctx, inst)
	if inst.On {
		t.Fatal("expected HVAC off at setpoint")
	}

	inst.Mem[0] = 20.5
	ctx.OutdoorC = 25
	b.Step(ctx, inst, sim.MsPerHour)
	if inst.On {
		t.Fatal("20.5 is inside ±1 deadband of 20; should be off")
	}
	// leak: T += -0.25*(20.5-25) = +1.125 → 21.625
	want := 20.5 - appliance.HVACLeakPerHour*(20.5-25)
	if math.Abs(appliance.IndoorC(inst)-want) > 1e-9 {
		t.Fatalf("drift indoor = %v, want %v", appliance.IndoorC(inst), want)
	}
}

func TestColderOutdoorSapsFaster(t *testing.T) {
	b := appliance.Lookup(appliance.KindHVAC)
	step := func(outdoor float64) float64 {
		inst := &appliance.Instance{Kind: appliance.KindHVAC, RatedPKw: 2.5, Mem: [8]float64{20, 20}}
		ctx := appliance.MakeContext(sim.EpochMs, outdoor, nil)
		b.Step(ctx, inst, sim.MsPerHour)
		return 20 - appliance.IndoorC(inst) // °C lost in 1h while off
	}
	mild := step(15)
	cold := step(5)
	if !(cold > mild && mild > 0) {
		t.Fatalf("expected cold loss %v > mild loss %v > 0", cold, mild)
	}
	// Exact: loss = Leak * (20 - outdoor)
	if math.Abs(mild-appliance.HVACLeakPerHour*5) > 1e-9 {
		t.Fatalf("mild loss = %v, want %v", mild, appliance.HVACLeakPerHour*5)
	}
	if math.Abs(cold-appliance.HVACLeakPerHour*15) > 1e-9 {
		t.Fatalf("cold loss = %v, want %v", cold, appliance.HVACLeakPerHour*15)
	}
}

func TestHVACCoolingAgainstHotOutdoor(t *testing.T) {
	b := appliance.Lookup(appliance.KindHVAC)
	inst := &appliance.Instance{Kind: appliance.KindHVAC, RatedPKw: 2.5, Mem: [8]float64{23, 20}}
	ctx := appliance.MakeContext(sim.EpochMs, 25, nil)
	b.Step(ctx, inst, sim.MsPerHour)
	if !inst.On {
		t.Fatal("expected cooling on at 23 °C")
	}
	// T += -0.25*(23-25) - 2.5 = 0.5 - 2.5 = -2 → 21
	want := 23 - appliance.HVACLeakPerHour*(23-25) - appliance.HVACDriveCPerHour
	if math.Abs(appliance.IndoorC(inst)-want) > 1e-9 {
		t.Fatalf("cool step indoor = %v, want %v", appliance.IndoorC(inst), want)
	}
}

func TestHVACSetpointsAreGaussianAroundMean(t *testing.T) {
	b := appliance.Lookup(appliance.KindHVAC)
	rng := rand.New(rand.NewSource(42))
	const n = 500
	var sum, sumSq float64
	for i := 0; i < n; i++ {
		inst := &appliance.Instance{Kind: appliance.KindHVAC, RatedPKw: 2.5}
		ctx := appliance.MakeContext(sim.EpochMs, 20, rng)
		b.Init(ctx, inst)
		s := appliance.SetpointC(inst)
		sum += s
		sumSq += s * s
	}
	mean := sum / n
	variance := sumSq/n - mean*mean
	std := math.Sqrt(variance)
	if math.Abs(mean-appliance.HVACSetpointMeanC) > 0.3 {
		t.Fatalf("sample mean = %v, want ~%v", mean, appliance.HVACSetpointMeanC)
	}
	if math.Abs(std-appliance.HVACSetpointSigmaC) > 0.4 {
		t.Fatalf("sample std = %v, want ~%v", std, appliance.HVACSetpointSigmaC)
	}
}
