package wiring_test

import (
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/grid"
	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/systems/loadflow"
	"github.com/cstevenson98/energy-tycoon/game/systems/loadtick"
	"github.com/cstevenson98/energy-tycoon/game/systems/wiring"
	"github.com/cstevenson98/gowasm-engine/pkg/ecs"
)

func TestAttachDetachAndLoadflowPipeline(t *testing.T) {
	w := ecs.NewWorld()
	occ := grid.NewGridOccupancy()
	net := network.NewElectricalNetwork()
	ecs.SetResource(w, occ)
	ecs.SetResource(w, net)

	cGen := grid.GridCoord{Col: 0, Row: 0}
	cGhost := grid.GridCoord{Col: 1, Row: 0} // gen port — shares gen bus
	cMid := grid.GridCoord{Col: 2, Row: 0}
	cHouse := grid.GridCoord{Col: 3, Row: 0} // not adjacent to cGhost

	gen := grid.SpawnGenerator(w, cGen)
	house := grid.SpawnHouse(w, cHouse)
	occ.Occupy(cGen, gen)
	occ.Occupy(cHouse, house)

	wiring.Attach(w, gen, grid.ToolGenerator, cGen, occ)
	wiring.Attach(w, house, grid.ToolHouse, cHouse, occ)

	// Line from gen ghost port to house: 2 buses, 1 series branch.
	line := grid.SpawnLine(w, []grid.GridCoord{cGhost, cMid, cHouse})
	occ.Occupy(cMid, line)
	wiring.AttachLine(w, line, occ)

	if len(net.Buses()) != 2 {
		t.Fatalf("buses = %d, want 2 (gen+house; ghost is not a bus)", len(net.Buses()))
	}
	if len(net.Branches()) != 1 {
		t.Fatalf("branches = %d, want 1", len(net.Branches()))
	}

	// All four gen ports resolve to the same bus.
	genBus, ok := net.BusForEntity(gen)
	if !ok {
		t.Fatal("gen bus missing")
	}
	for _, port := range grid.CardinalNeighbours(cGen) {
		if occ.Occupied(port) {
			continue
		}
		b, ok := wiring.ResolveBus(w, occ, port)
		if !ok || b.ID != genBus.ID {
			t.Fatalf("port %+v bus=%v, want gen bus %d", port, b, genBus.ID)
		}
	}

	lf := loadflow.NewLoadflowSystem(w)
	lf.Update(w, 0)
	if !net.State.Converged {
		t.Fatalf("expected converged, LastError=%q", net.State.LastError)
	}

	tick := loadtick.NewLoadTickSystem(w)
	tick.IntervalMs = 1
	tick.Update(w, 2)
	lf.Update(w, 0)

	wiring.Detach(w, line)
	if len(net.Branches()) != 0 {
		t.Fatalf("branches after line detach = %d, want 0", len(net.Branches()))
	}
}

func TestHouseGhostPortsShareHouseBus(t *testing.T) {
	w := ecs.NewWorld()
	occ := grid.NewGridOccupancy()
	net := network.NewElectricalNetwork()
	ecs.SetResource(w, occ)
	ecs.SetResource(w, net)

	c := grid.GridCoord{Col: 5, Row: 5}
	house := grid.SpawnHouse(w, c)
	occ.Occupy(c, house)
	wiring.Attach(w, house, grid.ToolHouse, c, occ)
	hb, _ := net.BusForEntity(house)

	n := 0
	for _, port := range grid.CardinalNeighbours(c) {
		if !grid.IsDeviceGhostPort(w, occ, port) {
			t.Fatalf("expected ghost port at %+v", port)
		}
		b, ok := wiring.ResolveBus(w, occ, port)
		if !ok || b.ID != hb.ID {
			t.Fatalf("house port %+v resolved to %v, want bus %d", port, b, hb.ID)
		}
		n++
	}
	if n != 4 {
		t.Fatalf("ghost ports=%d, want 4", n)
	}
}
