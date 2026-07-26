package network_test

import (
	"math"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/network"
)

func stampV(n *network.ElectricalNetwork, id network.BusID, v float64) {
	bs, ok := n.BusStateFor(id)
	if !ok {
		return
	}
	bs.Result.VoltMag = v
}

func almostEqual(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestVoltageProfilesLinear(t *testing.T) {
	net, w := emptyNet()
	g := mustAddBus(t, net, newEntity(w), network.BusGenerator)
	a := mustAddBus(t, net, newEntity(w), network.BusJunction)
	b := mustAddBus(t, net, newEntity(w), network.BusLoad)

	r1, r2 := 0.1, 0.2
	brGA := net.AddBranch(g.ID, a.ID, r1, 0)
	brAB := net.AddBranch(a.ID, b.ID, r2, 0)

	stampV(net, g.ID, 230)
	stampV(net, a.ID, 220)
	stampV(net, b.ID, 210)

	feeders := net.VoltageProfiles(g.ID)
	if len(feeders) != 1 {
		t.Fatalf("feeders = %d, want 1", len(feeders))
	}
	fp := feeders[0]
	if fp.RootBranch != brGA.ID {
		t.Fatalf("RootBranch = %d, want %d", fp.RootBranch, brGA.ID)
	}
	if len(fp.Segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(fp.Segments))
	}

	s0, s1 := fp.Segments[0], fp.Segments[1]
	almostEqual(t, s0.DistFrom, 0)
	almostEqual(t, s0.DistTo, r1)
	almostEqual(t, s0.VFrom, 230)
	almostEqual(t, s0.VTo, 220)

	almostEqual(t, s1.DistFrom, r1)
	almostEqual(t, s1.DistTo, r1+r2)
	almostEqual(t, s1.VFrom, 220)
	almostEqual(t, s1.VTo, 210)

	_ = brAB
}

func TestVoltageProfilesFork(t *testing.T) {
	net, w := emptyNet()
	g := mustAddBus(t, net, newEntity(w), network.BusGenerator)
	a := mustAddBus(t, net, newEntity(w), network.BusJunction)
	b := mustAddBus(t, net, newEntity(w), network.BusLoad)
	c := mustAddBus(t, net, newEntity(w), network.BusLoad)

	rGA, rAB, rAC := 0.1, 0.2, 0.3
	brGA := net.AddBranch(g.ID, a.ID, rGA, 0)
	net.AddBranch(a.ID, b.ID, rAB, 0)
	net.AddBranch(a.ID, c.ID, rAC, 0)

	stampV(net, g.ID, 230)
	stampV(net, a.ID, 225)
	stampV(net, b.ID, 220)
	stampV(net, c.ID, 215)

	feeders := net.VoltageProfiles(g.ID)
	if len(feeders) != 1 {
		t.Fatalf("feeders = %d, want 1", len(feeders))
	}
	fp := feeders[0]
	if fp.RootBranch != brGA.ID {
		t.Fatalf("RootBranch = %d, want %d", fp.RootBranch, brGA.ID)
	}
	// Root G—A plus two downstream legs from A.
	if len(fp.Segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(fp.Segments))
	}
	almostEqual(t, fp.Segments[0].DistFrom, 0)
	almostEqual(t, fp.Segments[0].DistTo, rGA)

	// Downstream segments share DistFrom at A.
	for _, s := range fp.Segments[1:] {
		almostEqual(t, s.DistFrom, rGA)
	}
}

func TestVoltageProfilesTwoRoots(t *testing.T) {
	net, w := emptyNet()
	g := mustAddBus(t, net, newEntity(w), network.BusGenerator)
	a := mustAddBus(t, net, newEntity(w), network.BusLoad)
	b := mustAddBus(t, net, newEntity(w), network.BusLoad)

	brGA := net.AddBranch(g.ID, a.ID, 0.1, 0)
	brGB := net.AddBranch(g.ID, b.ID, 0.2, 0)

	stampV(net, g.ID, 230)
	stampV(net, a.ID, 220)
	stampV(net, b.ID, 210)

	feeders := net.VoltageProfiles(g.ID)
	if len(feeders) != 2 {
		t.Fatalf("feeders = %d, want 2", len(feeders))
	}
	if feeders[0].RootBranch != brGA.ID || feeders[1].RootBranch != brGB.ID {
		t.Fatalf("roots = [%d %d], want [%d %d]",
			feeders[0].RootBranch, feeders[1].RootBranch, brGA.ID, brGB.ID)
	}
	if len(feeders[0].Segments) != 1 || len(feeders[1].Segments) != 1 {
		t.Fatalf("segment lens = %d %d, want 1 1",
			len(feeders[0].Segments), len(feeders[1].Segments))
	}
}

func TestVoltageProfilesLoop(t *testing.T) {
	// G—A—B—C—A: closing edge C—A must not revisit A.
	net, w := emptyNet()
	g := mustAddBus(t, net, newEntity(w), network.BusGenerator)
	a := mustAddBus(t, net, newEntity(w), network.BusJunction)
	b := mustAddBus(t, net, newEntity(w), network.BusJunction)
	c := mustAddBus(t, net, newEntity(w), network.BusLoad)

	net.AddBranch(g.ID, a.ID, 0.1, 0)
	net.AddBranch(a.ID, b.ID, 0.1, 0)
	net.AddBranch(b.ID, c.ID, 0.1, 0)
	net.AddBranch(c.ID, a.ID, 0.1, 0)

	stampV(net, g.ID, 230)
	stampV(net, a.ID, 220)
	stampV(net, b.ID, 210)
	stampV(net, c.ID, 200)

	feeders := net.VoltageProfiles(g.ID)
	if len(feeders) != 1 {
		t.Fatalf("feeders = %d, want 1", len(feeders))
	}
	// Spanning tree: G—A, A—B, B—C (not C—A).
	if len(feeders[0].Segments) != 3 {
		t.Fatalf("segments = %d, want 3 (loop edge omitted)", len(feeders[0].Segments))
	}
}

func TestVoltageProfilesNonGenerator(t *testing.T) {
	net, w := emptyNet()
	load := mustAddBus(t, net, newEntity(w), network.BusLoad)
	if got := net.VoltageProfiles(load.ID); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
