package appliance_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/appliance"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
)

func TestDiurnalBaseColdAtDawnWarmAfternoon(t *testing.T) {
	dawn := appliance.DiurnalBaseC(appliance.OutdoorMinHour / 24.0)
	wantCold := appliance.OutdoorMeanC - appliance.OutdoorAmplitudeC
	if math.Abs(dawn-wantCold) > 1e-9 {
		t.Fatalf("dawn = %v, want %v", dawn, wantCold)
	}

	afternoon := appliance.DiurnalBaseC(appliance.OutdoorMinHour/24.0 + 0.5)
	wantWarm := appliance.OutdoorMeanC + appliance.OutdoorAmplitudeC
	if math.Abs(afternoon-wantWarm) > 1e-9 {
		t.Fatalf("afternoon = %v, want %v", afternoon, wantWarm)
	}
}

func TestAmbientAdvanceFollowsDayCycle(t *testing.T) {
	// Deterministic: zero-ish noise by using a fixed rng and checking near base.
	a := appliance.NewAmbientTemp()
	rng := rand.New(rand.NewSource(1))

	// Mid-afternoon peak hour.
	peakMs := sim.EpochMs + int64((appliance.OutdoorMinHour+12)*float64(sim.MsPerHour))
	a.Advance(peakMs, rng)
	base := appliance.DiurnalBaseC(sim.DayFraction(peakMs))
	if math.Abs(a.OutdoorC-base) > 3*appliance.OutdoorNoiseSigmaC {
		t.Fatalf("OutdoorC=%v far from base %v", a.OutdoorC, base)
	}
}

func TestAmbientNoiseChangesOverTime(t *testing.T) {
	a := appliance.NewAmbientTemp()
	rng := rand.New(rand.NewSource(99))
	a.Advance(sim.EpochMs, rng)
	first := a.OutdoorC
	// Same clock time next day → same base; noise should usually differ.
	a.Advance(sim.EpochMs+sim.MsPerDay, rng)
	second := a.OutdoorC
	base1 := appliance.DiurnalBaseC(0)
	base2 := appliance.DiurnalBaseC(0)
	if base1 != base2 {
		t.Fatal("base should match at same day fraction")
	}
	if first == second {
		// Extremely unlikely with OU over 24h; if it happens, step more.
		a.Advance(sim.EpochMs+2*sim.MsPerDay, rng)
		if a.OutdoorC == first {
			t.Fatal("expected noise to change outdoor over days")
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, "now"},
		{30 * sim.MsPerSecond, "30s"},
		{5 * sim.MsPerMinute, "5m"},
		{2 * sim.MsPerHour, "2h"},
		{2*sim.MsPerHour + 15*sim.MsPerMinute, "2h15m"},
	}
	for _, tc := range cases {
		if got := appliance.FormatDuration(tc.ms); got != tc.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}
