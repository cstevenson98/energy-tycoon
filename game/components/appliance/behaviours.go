package appliance

import (
	"math"
	"math/rand"

	"github.com/cstevenson98/energy-tycoon/game/components/sim"
)

func init() {
	Register(alwaysOnBehaviour{})
	Register(fridgeBehaviour{})
	Register(hvacBehaviour{})
}

// --- always_on ---

type alwaysOnBehaviour struct{}

func (alwaysOnBehaviour) Kind() ApplianceKind { return KindAlwaysOn }

func (alwaysOnBehaviour) Init(_ Context, inst *Instance) {
	inst.On = true
	if inst.RatedPKw <= 0 {
		inst.RatedPKw = 0.1
	}
	if inst.RatedQKw <= 0 {
		inst.RatedQKw = ratedQFromP(inst.RatedPKw)
	}
}

func (alwaysOnBehaviour) Step(_ Context, inst *Instance, _ int64) {
	inst.On = true
}

func (alwaysOnBehaviour) PowerKW(inst *Instance) (float64, float64) {
	if !inst.On {
		return 0, 0
	}
	return inst.RatedPKw, inst.RatedQKw
}

// --- fridge ---

const (
	fridgeOnMinMs  = 15 * sim.MsPerMinute
	fridgeOnMaxMs  = 30 * sim.MsPerMinute
	fridgeOffMinMs = 20 * sim.MsPerMinute
	fridgeOffMaxMs = 40 * sim.MsPerMinute
	memFridgeTimer = 0 // remaining ms until toggle
)

type fridgeBehaviour struct{}

func (fridgeBehaviour) Kind() ApplianceKind { return KindFridge }

func (fridgeBehaviour) Init(ctx Context, inst *Instance) {
	if inst.RatedPKw <= 0 {
		inst.RatedPKw = 0.15
	}
	if inst.RatedQKw <= 0 {
		inst.RatedQKw = ratedQFromP(inst.RatedPKw)
	}
	inst.On = false
	inst.Mem[memFridgeTimer] = float64(fridgeRandDuration(ctx, fridgeOffMinMs, fridgeOffMaxMs))
}

func (fridgeBehaviour) Step(ctx Context, inst *Instance, dtMs int64) {
	inst.Mem[memFridgeTimer] -= float64(dtMs)
	for inst.Mem[memFridgeTimer] <= 0 {
		inst.On = !inst.On
		var dur int64
		if inst.On {
			dur = fridgeRandDuration(ctx, fridgeOnMinMs, fridgeOnMaxMs)
		} else {
			dur = fridgeRandDuration(ctx, fridgeOffMinMs, fridgeOffMaxMs)
		}
		inst.Mem[memFridgeTimer] += float64(dur)
	}
}

func (fridgeBehaviour) PowerKW(inst *Instance) (float64, float64) {
	if !inst.On {
		return 0, 0
	}
	return inst.RatedPKw, inst.RatedQKw
}

// FridgeTimerMs returns ms until the next fridge toggle, or 0 if not a fridge.
func FridgeTimerMs(inst *Instance) int64 {
	if inst == nil || inst.Kind != KindFridge {
		return 0
	}
	return int64(inst.Mem[memFridgeTimer])
}

func fridgeRandDuration(ctx Context, minMs, maxMs int64) int64 {
	span := maxMs - minMs
	if span <= 0 {
		return minMs
	}
	if ctx.Rand != nil {
		return minMs + ctx.Rand.Int63n(span+1)
	}
	return minMs + span/2
}

// --- hvac ---

const (
	// HVACSetpointMeanC is the population mean thermostat target (°C).
	HVACSetpointMeanC = 20.0
	// HVACSetpointSigmaC is the Gaussian std-dev of per-house setpoints (°C).
	HVACSetpointSigmaC = 2.0
	// HVACDeadbandC is the ± band around the setpoint where HVAC stays off.
	HVACDeadbandC = 1.0
	// HVACLeakPerHour is Newton's-law leak strength (1/h): dT/dt includes
	// -Leak*(Tindoor - Tout). Larger |ΔT| (colder or hotter outdoors) saps faster.
	HVACLeakPerHour = 0.25
	// HVACDriveCPerHour is heating/cooling push when the unit is on (°C/h).
	HVACDriveCPerHour = 2.5

	memIndoorC   = 0
	memSetpointC = 1
)

type hvacBehaviour struct{}

func (hvacBehaviour) Kind() ApplianceKind { return KindHVAC }

func (hvacBehaviour) Init(ctx Context, inst *Instance) {
	if inst.RatedPKw <= 0 {
		inst.RatedPKw = 2.5
	}
	if inst.RatedQKw <= 0 {
		inst.RatedQKw = ratedQFromP(inst.RatedPKw)
	}
	// Mem[memSetpointC] == 0 means unset; sample N(mean, σ²). Tests may pre-set it.
	if inst.Mem[memSetpointC] == 0 {
		inst.Mem[memSetpointC] = drawHVACSetpoint(ctx.Rand)
	}
	inst.Mem[memIndoorC] = ctx.OutdoorC
	inst.On = hvacShouldRun(inst.Mem[memIndoorC], SetpointC(inst))
}

func (hvacBehaviour) Step(ctx Context, inst *Instance, dtMs int64) {
	t := inst.Mem[memIndoorC]
	set := SetpointC(inst)
	cooling := t > set+HVACDeadbandC
	heating := t < set-HVACDeadbandC
	inst.On = cooling || heating

	dtH := float64(dtMs) / float64(sim.MsPerHour)
	// Leak always pulls toward outdoor; bigger gap ⇒ faster drift.
	t += -HVACLeakPerHour * (t - ctx.OutdoorC) * dtH
	if heating {
		t += HVACDriveCPerHour * dtH
	} else if cooling {
		t -= HVACDriveCPerHour * dtH
	}
	inst.Mem[memIndoorC] = t
}

func (hvacBehaviour) PowerKW(inst *Instance) (float64, float64) {
	if !inst.On {
		return 0, 0
	}
	return inst.RatedPKw, inst.RatedQKw
}

func hvacShouldRun(t, set float64) bool {
	return t > set+HVACDeadbandC || t < set-HVACDeadbandC
}

func drawHVACSetpoint(rng *rand.Rand) float64 {
	var z float64
	if rng != nil {
		z = rng.NormFloat64()
	} else {
		z = rand.NormFloat64()
	}
	return HVACSetpointMeanC + HVACSetpointSigmaC*z
}

// IndoorC returns the HVAC indoor temperature from Mem, or NaN if not HVAC.
func IndoorC(inst *Instance) float64 {
	if inst == nil || inst.Kind != KindHVAC {
		return math.NaN()
	}
	return inst.Mem[memIndoorC]
}

// SetpointC returns the instance thermostat target (°C), or the population mean
// if unset / not an HVAC.
func SetpointC(inst *Instance) float64 {
	if inst == nil || inst.Kind != KindHVAC {
		return HVACSetpointMeanC
	}
	if inst.Mem[memSetpointC] == 0 {
		return HVACSetpointMeanC
	}
	return inst.Mem[memSetpointC]
}
