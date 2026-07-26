package appliance

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/cstevenson98/energy-tycoon/game/components/sim"
)

const (
	// OutdoorMeanC is the daily-average outdoor temperature (°C).
	OutdoorMeanC = 20.0
	// OutdoorAmplitudeC is half the clean diurnal swing (°C): coldest ≈ mean−amp, warmest ≈ mean+amp.
	OutdoorAmplitudeC = 5.0
	// OutdoorMinHour is the hour of day (UTC) when the clean curve is coldest (~dawn).
	OutdoorMinHour = 5.0
	// OutdoorNoiseSigmaC is the steady-state std-dev of the OU noise (°C).
	OutdoorNoiseSigmaC = 0.8
	// OutdoorNoiseTauHours is the noise correlation time (hours).
	OutdoorNoiseTauHours = 3.0
)

// AmbientTemp is the world resource for outdoor air temperature.
// OutdoorC follows a diurnal sine wave plus slowly varying noise.
type AmbientTemp struct {
	OutdoorC float64
	noise    float64
	lastMs   int64
}

// NewAmbientTemp evaluates the day-cycle at the sim epoch (no noise yet).
func NewAmbientTemp() *AmbientTemp {
	a := &AmbientTemp{lastMs: sim.EpochMs}
	a.OutdoorC = DiurnalBaseC(sim.DayFraction(sim.EpochMs))
	return a
}

// DiurnalBaseC returns the clean outdoor temperature (°C) for dayFraction in [0, 1).
// Coldest near OutdoorMinHour, warmest ~12 hours later.
func DiurnalBaseC(dayFraction float64) float64 {
	f := dayFraction - math.Floor(dayFraction)
	phase := 2 * math.Pi * (f - OutdoorMinHour/24.0)
	return OutdoorMeanC - OutdoorAmplitudeC*math.Cos(phase)
}

// Advance updates OutdoorC for nowMs: diurnal base + Ornstein–Uhlenbeck noise.
// Returns true if OutdoorC changed.
func (a *AmbientTemp) Advance(nowMs int64, rng *rand.Rand) bool {
	if a == nil {
		return false
	}
	prev := a.OutdoorC
	dtMs := nowMs - a.lastMs
	if dtMs < 0 {
		dtMs = 0
	}
	if dtMs > 0 {
		a.noise = evolveNoise(a.noise, float64(dtMs)/float64(sim.MsPerHour), rng)
		a.lastMs = nowMs
	}
	a.OutdoorC = DiurnalBaseC(sim.DayFraction(nowMs)) + a.noise
	return a.OutdoorC != prev
}

func evolveNoise(noise, dtHours float64, rng *rand.Rand) float64 {
	if dtHours <= 0 {
		return noise
	}
	// Cap a single jump so fast-forward does not explode the OU step.
	if dtHours > OutdoorNoiseTauHours*4 {
		dtHours = OutdoorNoiseTauHours * 4
	}
	alpha := math.Exp(-dtHours / OutdoorNoiseTauHours)
	sigma := OutdoorNoiseSigmaC * math.Sqrt(math.Max(0, 1-alpha*alpha))
	return alpha*noise + sigma*gauss(rng)
}

func gauss(rng *rand.Rand) float64 {
	if rng != nil {
		return rng.NormFloat64()
	}
	return rand.NormFloat64()
}

// FormatDuration formats a positive sim-ms duration for UI (e.g. "15m", "1h").
func FormatDuration(ms int64) string {
	if ms <= 0 {
		return "now"
	}
	if ms < sim.MsPerMinute {
		return fmt.Sprintf("%ds", ms/sim.MsPerSecond)
	}
	if ms < sim.MsPerHour {
		return fmt.Sprintf("%dm", ms/sim.MsPerMinute)
	}
	h := ms / sim.MsPerHour
	m := (ms % sim.MsPerHour) / sim.MsPerMinute
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}
