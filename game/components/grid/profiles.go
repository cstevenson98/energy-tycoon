package grid

import (
	"math"
	"math/rand"
)

const (
	// PeakKWMin / PeakKWMax bound the uniform random peak assigned at spawn.
	PeakKWMin = 3.0
	PeakKWMax = 7.0

	// LoadPowerFactor is the lagging PF used to derive Q from P.
	LoadPowerFactor = 0.95
)

// ProfileID names a demand shape in the registry.
type ProfileID string

const ProfileSummerResidential ProfileID = "summer_residential"

// Profile is a named daily demand shape. Hourly[h] is the normalized
// multiplier at hour h (UTC), in [0, 1]. Values between hours are linearly
// interpolated; the day wraps from hour 23 back to hour 0.
type Profile struct {
	ID     ProfileID
	Name   string
	Hourly [24]float64
}

// summerResidentialHourly is a typical summer residential shape: low overnight,
// morning bump, afternoon trough, evening peak at 1.0 around 19:00.
var summerResidentialHourly = [24]float64{
	0.25, 0.22, 0.20, 0.18, 0.18, 0.22, // 00–05
	0.35, 0.55, 0.70, 0.55, 0.45, 0.40, // 06–11
	0.42, 0.45, 0.50, 0.55, 0.65, 0.80, // 12–17
	0.95, 1.00, 0.90, 0.70, 0.50, 0.35, // 18–23
}

var profiles = map[ProfileID]Profile{
	ProfileSummerResidential: {
		ID:     ProfileSummerResidential,
		Name:   "Summer residential",
		Hourly: summerResidentialHourly,
	},
}

// LookupProfile returns the registered profile for id, or the summer residential
// default when id is unknown.
func LookupProfile(id ProfileID) Profile {
	if p, ok := profiles[id]; ok {
		return p
	}
	return profiles[ProfileSummerResidential]
}

// Multiplier returns the interpolated shape value at dayFraction in [0, 1).
// Inputs outside [0, 1) are wrapped into that range.
func (p Profile) Multiplier(dayFraction float64) float64 {
	f := dayFraction
	f = f - math.Floor(f)
	hourF := f * 24
	h0 := int(hourF) % 24
	h1 := (h0 + 1) % 24
	t := hourF - float64(int(hourF))
	return p.Hourly[h0]*(1-t) + p.Hourly[h1]*t
}

// qFromP returns lagging reactive demand for active P at LoadPowerFactor.
func qFromP(pKW float64) float64 {
	if LoadPowerFactor <= 0 || LoadPowerFactor >= 1 {
		return 0
	}
	return pKW * math.Tan(math.Acos(LoadPowerFactor))
}

// DemandKW evaluates profileID at dayFraction, scaled by peakKW.
// Unknown profiles fall back to summer residential.
func DemandKW(profileID ProfileID, peakKW, dayFraction float64) (pKW, qKW float64) {
	m := LookupProfile(profileID).Multiplier(dayFraction)
	pKW = peakKW * m
	qKW = qFromP(pKW)
	return pKW, qKW
}

// RandPeakKW returns a uniform random peak in [PeakKWMin, PeakKWMax].
func RandPeakKW() float64 {
	return PeakKWMin + (PeakKWMax-PeakKWMin)*rand.Float64()
}
