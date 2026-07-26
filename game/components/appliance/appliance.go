// Package appliance defines fixed-tick appliance behaviours that drive
// house demand as an alternative to load profiles.
package appliance

import (
	"math"
	"math/rand"

	"github.com/cstevenson98/energy-tycoon/game/components/sim"
)

// ApplianceKind identifies a registered Behaviour.
type ApplianceKind string

const (
	KindFridge   ApplianceKind = "fridge"
	KindAlwaysOn ApplianceKind = "always_on"
	KindHVAC     ApplianceKind = "hvac"
)

// Instance is one appliance attached to a house.
type Instance struct {
	Kind     ApplianceKind
	On       bool
	RatedPKw float64
	RatedQKw float64
	// Mem holds behaviour-specific state (HVAC indoor °C, fridge timer, …).
	Mem   [8]float64
	Flags uint32
}

// HouseAppliances is the ECS component listing appliances on a house entity.
type HouseAppliances struct {
	Items []Instance
}

// Context is passed into Behaviour methods.
type Context struct {
	NowMs    int64
	DayFrac  float64
	OutdoorC float64
	Rand     *rand.Rand
}

// Behaviour is a pluggable appliance strategy advanced on a fixed sim tick.
type Behaviour interface {
	Kind() ApplianceKind
	Init(ctx Context, inst *Instance)
	// Step advances inst by dtMs of sim time.
	Step(ctx Context, inst *Instance, dtMs int64)
	PowerKW(inst *Instance) (pKW, qKW float64)
}

var registry = map[ApplianceKind]Behaviour{}

// Register adds b to the global behaviour registry. Panics on duplicate kind.
func Register(b Behaviour) {
	if b == nil {
		panic("appliance: Register nil Behaviour")
	}
	k := b.Kind()
	if _, ok := registry[k]; ok {
		panic("appliance: duplicate Behaviour kind " + string(k))
	}
	registry[k] = b
}

// Lookup returns the Behaviour for k, or nil if unregistered.
func Lookup(k ApplianceKind) Behaviour {
	return registry[k]
}

// AggregatePower sums PowerKW across all appliances.
func AggregatePower(ha *HouseAppliances) (pKW, qKW float64) {
	if ha == nil {
		return 0, 0
	}
	for i := range ha.Items {
		b := Lookup(ha.Items[i].Kind)
		if b == nil {
			continue
		}
		p, q := b.PowerKW(&ha.Items[i])
		pKW += p
		qKW += q
	}
	return pKW, qKW
}

// StepAll runs Step on every appliance in ha.
func StepAll(ctx Context, ha *HouseAppliances, dtMs int64) {
	if ha == nil || dtMs <= 0 {
		return
	}
	for i := range ha.Items {
		if b := Lookup(ha.Items[i].Kind); b != nil {
			b.Step(ctx, &ha.Items[i], dtMs)
		}
	}
}

// InitInstance looks up the behaviour and runs Init; no-op if unknown kind.
func InitInstance(ctx Context, inst *Instance) {
	if b := Lookup(inst.Kind); b != nil {
		b.Init(ctx, inst)
	}
}

// ratedQFromP derives lagging Q at PF 0.95 (matches grid.LoadPowerFactor).
func ratedQFromP(pKW float64) float64 {
	const pf = 0.95
	return pKW * math.Tan(math.Acos(pf))
}

// MakeContext builds a Context from clock/ambient fields.
func MakeContext(nowMs int64, outdoorC float64, rng *rand.Rand) Context {
	return Context{
		NowMs:    nowMs,
		DayFrac:  sim.DayFraction(nowMs),
		OutdoorC: outdoorC,
		Rand:     rng,
	}
}
