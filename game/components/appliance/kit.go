package appliance

import "math/rand"

// DefaultResidentialKit builds fridge + always_on + HVAC instances.
// Caller must run InitInstance on each item with a Context.
func DefaultResidentialKit(_ *rand.Rand) []Instance {
	return []Instance{
		{Kind: KindAlwaysOn, RatedPKw: 0.1},
		{Kind: KindFridge, RatedPKw: 0.15},
		{Kind: KindHVAC, RatedPKw: 2.5},
	}
}

// NewHouseAppliances builds a HouseAppliances from kit items and inits each
// behaviour.
func NewHouseAppliances(ctx Context, items []Instance) *HouseAppliances {
	ha := &HouseAppliances{Items: append([]Instance(nil), items...)}
	for i := range ha.Items {
		InitInstance(ctx, &ha.Items[i])
	}
	return ha
}
