package grid

import (
	"github.com/cstevenson98/energy-tycoon/game/components/appliance"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/milo/pkg/components"
	"github.com/cstevenson98/milo/pkg/ecs"
	"github.com/cstevenson98/milo/pkg/types"
)

// cellPosition returns a cell's top-left world position in pixels.
func cellPosition(cell GridCoord) types.Vector2 {
	ts := gameconfig.Global.TileSize
	return types.Vector2{X: float64(cell.Col) * ts, Y: float64(cell.Row) * ts}
}

// spawnTile builds a single-cell entity carrying Position, Sprite, a layer
// tag, Order and GridObject. Shared by SpawnGenerator/SpawnHouse;
// only texture/kind/layer/z differ between them.
func spawnTile(w *ecs.World, cell GridCoord, kind Tool, texturePath string, z int, background bool) ecs.Entity {
	ts := gameconfig.Global.TileSize
	pos := cellPosition(cell)
	size := types.Vector2{X: ts, Y: ts}

	sprite := &components.Sprite{
		TexturePath: texturePath,
		Size:        size,
		Columns:     1,
		Rows:        1,
		Visible:     true,
	}
	order := &components.Order{Z: z}

	var e ecs.Entity
	if background {
		m := ecs.NewMap4[components.Position, components.Sprite, components.LayerBackground, components.Order](w)
		e = m.NewEntity(&components.Position{X: pos.X, Y: pos.Y}, sprite, &components.LayerBackground{}, order)
	} else {
		m := ecs.NewMap4[components.Position, components.Sprite, components.LayerEntities, components.Order](w)
		e = m.NewEntity(&components.Position{X: pos.X, Y: pos.Y}, sprite, &components.LayerEntities{}, order)
	}

	ecs.NewMap1[GridObject](w).Add(e, &GridObject{Kind: kind, Cell: cell})
	return e
}

// SpawnGenerator spawns a generator tile at cell, on the ENTITIES layer.
// It also attaches a GeneratorProps component with a default 100 kW capacity.
func SpawnGenerator(w *ecs.World, cell GridCoord) ecs.Entity {
	e := spawnTile(w, cell, ToolGenerator, gameconfig.Global.GeneratorTexture, 0, false)
	ecs.NewMap1[GeneratorProps](w).Add(e, &GeneratorProps{MaxOutputKW: 100.0})
	return e
}

// SpawnHouse spawns a house tile at cell, on the ENTITIES layer.
// Demand is appliance-driven (fridge + always_on + HVAC). P/Q are the sum of
// the kit at the current sim time / outdoor temperature.
func SpawnHouse(w *ecs.World, cell GridCoord) ecs.Entity {
	e := spawnTile(w, cell, ToolHouse, gameconfig.Global.HouseTexture, 0, false)

	nowMs := sim.EpochMs
	if clock := ecs.GetResource[sim.SimClock](w); clock != nil {
		nowMs = clock.NowMs
	}
	outdoor := 20.0
	if amb := ecs.GetResource[appliance.AmbientTemp](w); amb != nil {
		outdoor = amb.OutdoorC
	}
	ctx := appliance.MakeContext(nowMs, outdoor, nil)
	ha := appliance.NewHouseAppliances(ctx, appliance.DefaultResidentialKit(nil))
	pKW, qKW := appliance.AggregatePower(ha)

	ecs.NewMap1[HouseLoad](w).Add(e, &HouseLoad{
		Source: DemandAppliances,
		PKw:    pKW,
		QKw:    qKW,
	})
	ecs.NewMap1[appliance.HouseAppliances](w).Add(e, ha)
	return e
}

// SpawnHouseWithProfile spawns a house using the summer residential load
// profile (DemandProfile). Kept for tests and profile-path exercises.
func SpawnHouseWithProfile(w *ecs.World, cell GridCoord) ecs.Entity {
	e := spawnTile(w, cell, ToolHouse, gameconfig.Global.HouseTexture, 0, false)
	peak := RandPeakKW()
	dayFrac := 0.0
	if clock := ecs.GetResource[sim.SimClock](w); clock != nil {
		dayFrac = sim.DayFraction(clock.NowMs)
	}
	pKW, qKW := DemandKW(ProfileSummerResidential, peak, dayFrac)
	ecs.NewMap1[HouseLoad](w).Add(e, &HouseLoad{
		Source:  DemandProfile,
		Profile: ProfileSummerResidential,
		PeakKW:  peak,
		PKw:     pKW,
		QKw:     qKW,
	})
	return e
}

// SpawnJunction spawns a junction node at cell. It has Position + GridObject
// but no visible sprite — overlays draw it as a circle.
func SpawnJunction(w *ecs.World, cell GridCoord) ecs.Entity {
	pos := cellPosition(cell)
	m := ecs.NewMap3[components.Position, components.LayerEntities, components.Order](w)
	e := m.NewEntity(
		&components.Position{X: pos.X, Y: pos.Y},
		&components.LayerEntities{},
		&components.Order{Z: 1},
	)
	ecs.NewMap1[GridObject](w).Add(e, &GridObject{Kind: ToolJunction, Cell: cell})
	return e
}

// SpawnLine spawns one polyline line entity covering path. No visible sprite —
// overlays draw a thick stroke through cell centres. LineSegmentProps R/X
// scale with max(1, len(path)-1) cell lengths. Caller must Occupy every cell
// in path with the returned entity, and ensure endpoint buses exist before
// wiring.AttachLine.
func SpawnLine(w *ecs.World, path []GridCoord) ecs.Entity {
	if len(path) == 0 {
		return ecs.Entity{}
	}
	cells := append([]GridCoord(nil), path...)
	pos := cellPosition(cells[0])
	m := ecs.NewMap3[components.Position, components.LayerEntities, components.Order](w)
	e := m.NewEntity(
		&components.Position{X: pos.X, Y: pos.Y},
		&components.LayerEntities{},
		&components.Order{Z: 0},
	)
	ecs.NewMap1[GridObject](w).Add(e, &GridObject{Kind: ToolLine, Cell: cells[0]})
	hops := len(cells) - 1
	if hops < 1 {
		hops = 1
	}
	ecs.NewMap1[LineSegmentProps](w).Add(e, &LineSegmentProps{
		ResistanceOhm: DefaultLineResistanceOhm * float64(hops),
		ReactanceOhm:  DefaultLineReactanceOhm * float64(hops),
	})
	ecs.NewMap1[LinePath](w).Add(e, &LinePath{Cells: cells})
	ecs.NewMap1[LineEndpoints](w).Add(e, &LineEndpoints{})
	return e
}

// SpawnLineSegment spawns a single-cell line (convenience for tests).
func SpawnLineSegment(w *ecs.World, cell GridCoord) ecs.Entity {
	return SpawnLine(w, []GridCoord{cell})
}

// SplitPathAt splits path at cell (must appear in path). Returns left and
// right sub-paths that both include cell as an endpoint. ok is false if cell
// is not on path or is already an endpoint (nothing to split).
func SplitPathAt(path []GridCoord, cell GridCoord) (left, right []GridCoord, ok bool) {
	idx := -1
	for i, c := range path {
		if c == cell {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, nil, false
	}
	if idx == 0 || idx == len(path)-1 {
		return nil, nil, false
	}
	left = append([]GridCoord(nil), path[:idx+1]...)
	right = append([]GridCoord(nil), path[idx:]...)
	return left, right, true
}

// ManhattanPath returns the cells of an L-shaped path from `from` to `to`:
// first horizontally along from.Row, then vertically along to.Col. Both
// endpoints are included exactly once (the corner cell is not duplicated).
func ManhattanPath(from, to GridCoord) []GridCoord {
	var path []GridCoord

	colStep := 1
	if to.Col < from.Col {
		colStep = -1
	}
	for c := from.Col; ; c += colStep {
		path = append(path, GridCoord{Col: c, Row: from.Row})
		if c == to.Col {
			break
		}
	}

	rowStep := 1
	if to.Row < from.Row {
		rowStep = -1
	}
	for r := from.Row; ; r += rowStep {
		if r == from.Row {
			if r == to.Row {
				break
			}
			continue
		}
		path = append(path, GridCoord{Col: to.Col, Row: r})
		if r == to.Row {
			break
		}
	}

	return path
}

