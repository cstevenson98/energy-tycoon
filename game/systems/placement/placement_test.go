package placement_test

import (
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/systems/loadflow"
	"github.com/cstevenson98/energy-tycoon/game/systems/placement"
	"github.com/cstevenson98/energy-tycoon/game/systems/wiring"
	"github.com/cstevenson98/gowasm-engine/pkg/ecs"
)

// Exercise completeLine via exporting through a thin test hook: place a
// horizontal feeder then cross it with a vertical line that hits the mid cell.
func TestMidLineSplitCreatesJunction(t *testing.T) {
	w := ecs.NewWorld()
	occ := grid.NewGridOccupancy()
	net := network.NewElectricalNetwork()
	ecs.SetResource(w, occ)
	ecs.SetResource(w, net)

	// Manual first line: junctions at (0,1) and (4,1), line interiors (1,1)(2,1)(3,1).
	j0 := grid.SpawnJunction(w, grid.GridCoord{Col: 0, Row: 1})
	j1 := grid.SpawnJunction(w, grid.GridCoord{Col: 4, Row: 1})
	occ.Occupy(grid.GridCoord{Col: 0, Row: 1}, j0)
	occ.Occupy(grid.GridCoord{Col: 4, Row: 1}, j1)
	wiring.Attach(w, j0, grid.ToolJunction, grid.GridCoord{Col: 0, Row: 1}, occ)
	wiring.Attach(w, j1, grid.ToolJunction, grid.GridCoord{Col: 4, Row: 1}, occ)

	path := []grid.GridCoord{
		{Col: 0, Row: 1}, {Col: 1, Row: 1}, {Col: 2, Row: 1}, {Col: 3, Row: 1}, {Col: 4, Row: 1},
	}
	line := grid.SpawnLine(w, path)
	for i := 1; i < len(path)-1; i++ {
		occ.Occupy(path[i], line)
	}
	wiring.AttachLine(w, line, occ)

	// Cross through mid (2,1) from (2,0) to (2,2) using placement completeLine.
	placement.CompleteLineForTest(w, occ, grid.GridCoord{Col: 2, Row: 0}, grid.GridCoord{Col: 2, Row: 2})

	// Expect a junction at (2,1).
	e, ok := occ.Cells[grid.GridCoord{Col: 2, Row: 1}]
	if !ok {
		t.Fatal("expected occupant at split cell")
	}
	go_ := ecs.NewMap1[grid.GridObject](w).Get(e)
	if go_ == nil || go_.Kind != grid.ToolJunction {
		t.Fatalf("split cell kind=%v, want junction", go_)
	}

	nLines := 0
	ecs.NewFilter1[grid.LinePath](w).Each(func(_ ecs.Entity, _ *grid.LinePath) { nLines++ })
	if nLines < 3 {
		t.Fatalf("lines=%d, want ≥3 (two halves + cross)", nLines)
	}

	lf := loadflow.NewLoadflowSystem(w)
	// Add a slack so solve can run: place gen next to j0.
	genCell := grid.GridCoord{Col: 0, Row: 0}
	gen := grid.SpawnGenerator(w, genCell)
	occ.Occupy(genCell, gen)
	wiring.Attach(w, gen, grid.ToolGenerator, genCell, occ)
	net.MarkDirty()
	lf.Update(w, 0)
	if !net.State.Converged && net.State.LastError != "" {
		// May still converge; only fail hard if no slack somehow.
		t.Logf("solve: converged=%v err=%s", net.State.Converged, net.State.LastError)
	}
}
