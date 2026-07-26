package appliance_test

import (
	"math/rand"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/appliance"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
)

func TestFridgeToggles(t *testing.T) {
	b := appliance.Lookup(appliance.KindFridge)
	rng := rand.New(rand.NewSource(7))
	inst := &appliance.Instance{Kind: appliance.KindFridge, RatedPKw: 0.15}
	ctx := appliance.MakeContext(sim.EpochMs, 20, rng)
	b.Init(ctx, inst)
	if inst.On {
		t.Fatal("fridge should start off")
	}
	timer := appliance.FridgeTimerMs(inst)
	if timer <= 0 {
		t.Fatalf("timer=%d, want > 0", timer)
	}
	b.Step(ctx, inst, timer)
	if !inst.On {
		t.Fatal("fridge should turn on after timer")
	}
	b.Step(ctx, inst, appliance.FridgeTimerMs(inst))
	if inst.On {
		t.Fatal("fridge should turn off after second cycle")
	}
}
