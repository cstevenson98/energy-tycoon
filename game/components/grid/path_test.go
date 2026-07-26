package grid_test

import (
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/grid"
)

func TestSplitPathAt(t *testing.T) {
	path := []grid.GridCoord{
		{Col: 0, Row: 0},
		{Col: 1, Row: 0},
		{Col: 2, Row: 0},
		{Col: 3, Row: 0},
	}
	left, right, ok := grid.SplitPathAt(path, grid.GridCoord{Col: 2, Row: 0})
	if !ok {
		t.Fatal("expected split ok")
	}
	if len(left) != 3 || left[0] != path[0] || left[2] != path[2] {
		t.Fatalf("left=%v", left)
	}
	if len(right) != 2 || right[0] != path[2] || right[1] != path[3] {
		t.Fatalf("right=%v", right)
	}
	if _, _, ok := grid.SplitPathAt(path, path[0]); ok {
		t.Fatal("endpoint should not split")
	}
	if _, _, ok := grid.SplitPathAt(path, grid.GridCoord{Col: 9, Row: 9}); ok {
		t.Fatal("missing cell should not split")
	}
}
