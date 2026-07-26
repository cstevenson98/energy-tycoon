// Package grid holds the ECS components and resources that describe the
// placement grid: cell addresses, placed-object kinds, occupancy tracking,
// and the placement UI state. It is the data layer for the grid game; all
// behaviour lives in the systems packages.
package grid

import "github.com/cstevenson98/gowasm-engine/pkg/ecs"

// Tool identifies which kind of object the player currently has selected to
// place (or ToolNone, meaning clicks select a cell for the inspector).
type Tool int

const (
	ToolNone Tool = iota
	ToolGenerator
	ToolHouse
	ToolLine
	ToolDelete
	ToolJunction // placed junction node (circle); not a toolbar tool
)

// Label returns the toolbar button label for the tool.
func (t Tool) Label() string {
	switch t {
	case ToolGenerator:
		return "GEN (G)"
	case ToolHouse:
		return "HOUSE (H)"
	case ToolLine:
		return "LINE (L)"
	case ToolDelete:
		return "DEL (X)"
	default:
		return "NONE"
	}
}

// GhostLetter is the single-character overlay drawn inside a placement ghost.
func (t Tool) GhostLetter() string {
	switch t {
	case ToolGenerator:
		return "G"
	case ToolHouse:
		return "H"
	case ToolLine:
		return "L"
	case ToolDelete:
		return "X"
	default:
		return "?"
	}
}

// KindLabel is a short inspector name for a placed GridObject.Kind.
func (t Tool) KindLabel() string {
	switch t {
	case ToolGenerator:
		return "generator"
	case ToolHouse:
		return "house"
	case ToolLine:
		return "line"
	case ToolJunction:
		return "junction"
	default:
		return "blank"
	}
}

// GridCoord is a cell address on the grid (column, row), independent of the
// grid's pixel size.
type GridCoord struct{ Col, Row int }

// GridObject marks a placed, single-cell entity - a generator, a house, or
// one tile of a line's path - and records which cell it occupies. Systems
// (and the renderer's own Position) key off Cell for occupancy/placement
// logic; Kind is informational (matches which sprite was spawned).
type GridObject struct {
	Kind Tool
	Cell GridCoord
}

// PlacementState is the per-World pointer/placement UI store: active tool,
// line-pending start, hover, and selection. Mutated by pointer + placement
// systems; read by GridState overlays / ImGui.
type PlacementState struct {
	Tool        Tool
	LinePending bool
	LineStart   GridCoord

	HoverCell    GridCoord // cell under the cursor (playfield)
	HoverValid   bool      // true when HoverCell is a valid grid cell
	HasSelection bool
	SelectedCell GridCoord
}

// HouseLoad is the power-demand component for a house entity. P and Q are
// in kilowatts / kilovars (kW, kVAR) and are always positive (consumed power).
// They are sampled at spawn and periodically re-sampled by LoadTickSystem.
type HouseLoad struct {
	PKw float64 // active power demand  [kW]
	QKw float64 // reactive power demand [kVAR]
}

// GeneratorProps holds the nameplate parameters of a generator entity.
// MaxOutputKW is the maximum active power the unit can deliver (kW).
type GeneratorProps struct {
	MaxOutputKW float64
}

// LineSegmentProps holds the electrical parameters of a line stroke.
// R/X are in ohms for the whole entity (see CellLengthM).
type LineSegmentProps struct {
	ResistanceOhm float64
	ReactanceOhm  float64
}

// LinePath lists every cell occupied by a polyline line entity. Occupancy
// maps each cell to the same entity; delete removes the whole stroke.
// Lines have no electrical bus — they are branches between endpoint buses.
type LinePath struct {
	Cells []GridCoord
}

// LineEndpoints records the network buses at the ends of a line after
// wiring.AttachLine, so Detach can remove the series branch reliably.
// IDs are network.BusID / BranchID stored as uint64 to avoid grid→network.
type LineEndpoints struct {
	FromBus  uint64
	ToBus    uint64
	BranchID uint64
	Wired    bool // true once AttachLine created a series branch
}

// CardinalNeighbours returns the four orthogonally adjacent cells.
func CardinalNeighbours(cell GridCoord) []GridCoord {
	return []GridCoord{
		{Col: cell.Col + 1, Row: cell.Row},
		{Col: cell.Col - 1, Row: cell.Row},
		{Col: cell.Col, Row: cell.Row + 1},
		{Col: cell.Col, Row: cell.Row - 1},
	}
}

// CellLengthM is the physical length represented by one grid cell / line tile.
const CellLengthM = 10.0 // metres

// CableOhmPerKm is a typical LV distribution feeder resistance
// (≈185 mm² aluminium, ~0.164 Ω/km). At 10 m/cell this supports ~100-bus
// radial feeders at LV without collapsing to unrealistically low voltages
// under a handful of house-scale loads.
const CableOhmPerKm = 0.164

// CableXPerKm is a small series reactance for the same LV cable class.
// Default 0 keeps bit-identical resistive behaviour until tuned; set a
// positive value (e.g. ~0.08 Ω/km) for angle/Q demos.
const CableXPerKm = 0.0

// DefaultLineResistanceOhm is R for one newly-placed line cell:
// CableOhmPerKm × (CellLengthM / 1000).
const DefaultLineResistanceOhm = CableOhmPerKm * CellLengthM / 1000.0 // 0.00164 Ω

// DefaultLineReactanceOhm is X for one newly-placed line cell.
const DefaultLineReactanceOhm = CableXPerKm * CellLengthM / 1000.0

// GridOccupancy is the per-World map of occupied cells, enforcing that
// generators, houses and line tiles never overlap each other.
type GridOccupancy struct {
	Cells map[GridCoord]ecs.Entity
}

// NewGridOccupancy creates an empty occupancy map.
func NewGridOccupancy() *GridOccupancy {
	return &GridOccupancy{Cells: make(map[GridCoord]ecs.Entity)}
}

// Occupied reports whether cell already holds a placed entity.
func (g *GridOccupancy) Occupied(cell GridCoord) bool {
	_, ok := g.Cells[cell]
	return ok
}

// Occupy records that e now occupies cell. Callers must check Occupied first.
func (g *GridOccupancy) Occupy(cell GridCoord, e ecs.Entity) {
	g.Cells[cell] = e
}
