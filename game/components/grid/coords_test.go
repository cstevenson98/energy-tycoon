package grid_test

import (
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/gameconfig"
	"github.com/cstevenson98/milo/pkg/components"
)

func TestScreenToCellAndRect(t *testing.T) {
	cfg := gameconfig.Global
	ts := cfg.TileSize
	cam := &components.Camera{X: 0, Y: 0, Zoom: 1}

	cell, ok := grid.ScreenToCell(cam, ts*3+1, ts*5+1)
	if !ok || cell != (grid.GridCoord{Col: 3, Row: 5}) {
		t.Fatalf("ScreenToCell = %+v %v, want (3,5) true", cell, ok)
	}

	_, ok = grid.ScreenToCell(cam, -ts, 0)
	if ok {
		t.Fatal("expected out-of-bounds left to be false")
	}
	_, ok = grid.ScreenToCell(cam, float64(cfg.GridCols)*ts+1, 0)
	if ok {
		t.Fatal("expected out-of-bounds right to be false")
	}

	x, y, w, h := grid.CellScreenRect(cam, grid.GridCoord{Col: 2, Row: 4})
	if x != 2*ts || y != 4*ts || w != ts || h != ts {
		t.Fatalf("CellScreenRect = %v,%v,%v,%v want %v,%v,%v,%v", x, y, w, h, 2*ts, 4*ts, ts, ts)
	}

	// Cell centre should map back to the same cell.
	cx, cy := x+w/2, y+h/2
	back, ok := grid.ScreenToCell(cam, cx, cy)
	if !ok || back != (grid.GridCoord{Col: 2, Row: 4}) {
		t.Fatalf("round-trip ScreenToCell = %+v %v", back, ok)
	}
}

func TestScreenToCellZoomAndPan(t *testing.T) {
	ts := gameconfig.Global.TileSize
	cam := &components.Camera{X: ts, Y: 2 * ts, Zoom: 2}

	// Screen (0,0) → world (ts, 2ts) → cell (1, 2)
	cell, ok := grid.ScreenToCell(cam, 0, 0)
	if !ok || cell != (grid.GridCoord{Col: 1, Row: 2}) {
		t.Fatalf("got %+v %v, want (1,2) true", cell, ok)
	}
}

func TestManhattanPath(t *testing.T) {
	from := grid.GridCoord{Col: 1, Row: 1}
	to := grid.GridCoord{Col: 1, Row: 1}
	p := grid.ManhattanPath(from, to)
	if len(p) != 1 || p[0] != from {
		t.Fatalf("same cell path = %v, want [%v]", p, from)
	}

	p = grid.ManhattanPath(grid.GridCoord{Col: 0, Row: 0}, grid.GridCoord{Col: 2, Row: 0})
	want := []grid.GridCoord{{0, 0}, {1, 0}, {2, 0}}
	if !coordsEq(p, want) {
		t.Fatalf("horizontal = %v, want %v", p, want)
	}

	p = grid.ManhattanPath(grid.GridCoord{Col: 0, Row: 0}, grid.GridCoord{Col: 2, Row: 2})
	// Horizontal along row 0, then vertical along col 2.
	want = []grid.GridCoord{{0, 0}, {1, 0}, {2, 0}, {2, 1}, {2, 2}}
	if !coordsEq(p, want) {
		t.Fatalf("L-shape = %v, want %v", p, want)
	}
}

func coordsEq(a, b []grid.GridCoord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
