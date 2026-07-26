package grid_test

import (
	"math"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/grid"
)

func TestSummerResidentialMultiplierMidnightAndPeak(t *testing.T) {
	p := grid.LookupProfile(grid.ProfileSummerResidential)

	midnight := p.Multiplier(0)
	if math.Abs(midnight-0.25) > 1e-9 {
		t.Fatalf("midnight multiplier = %v, want 0.25", midnight)
	}

	// Hour 19 is the peak knot (1.0).
	peak := p.Multiplier(19.0 / 24.0)
	if math.Abs(peak-1.0) > 1e-9 {
		t.Fatalf("19:00 multiplier = %v, want 1.0", peak)
	}
}

func TestDemandKWAtPeak(t *testing.T) {
	const peak = 5.0
	pKW, qKW := grid.DemandKW(grid.ProfileSummerResidential, peak, 19.0/24.0)
	if math.Abs(pKW-peak) > 1e-9 {
		t.Fatalf("PKw = %v, want %v", pKW, peak)
	}
	wantQ := peak * math.Tan(math.Acos(grid.LoadPowerFactor))
	if math.Abs(qKW-wantQ) > 1e-9 {
		t.Fatalf("QKw = %v, want %v", qKW, wantQ)
	}
}

func TestLookupProfileUnknownFallsBack(t *testing.T) {
	p := grid.LookupProfile("nope")
	if p.ID != grid.ProfileSummerResidential {
		t.Fatalf("fallback ID = %q, want %q", p.ID, grid.ProfileSummerResidential)
	}
}
