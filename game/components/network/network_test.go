package network_test

import (
	"errors"
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/milo/pkg/ecs"
)

func TestAddAndLookupBus(t *testing.T) {
	n := network.NewElectricalNetwork()
	w := ecs.NewWorld()
	e := ecs.NewMap1[struct{}](w).NewEntity(&struct{}{})

	bus := mustAddBus(t, n, e, network.BusGenerator)
	if bus.Type != network.BusGenerator {
		t.Fatalf("bus type = %v, want BusGenerator", bus.Type)
	}

	got, ok := n.BusForEntity(e)
	if !ok || got.ID != bus.ID {
		t.Fatalf("BusForEntity: got %v %v, want bus %v", got, ok, bus.ID)
	}
}

func TestAddBusDuplicateError(t *testing.T) {
	n := network.NewElectricalNetwork()
	w := ecs.NewWorld()
	e := ecs.NewMap1[struct{}](w).NewEntity(&struct{}{})
	mustAddBus(t, n, e, network.BusGenerator)

	_, err := n.AddBus(e, network.BusLoad)
	if !errors.Is(err, network.ErrDuplicateBus) {
		t.Fatalf("err = %v, want ErrDuplicateBus", err)
	}
}

func TestBranchAndNeighbors(t *testing.T) {
	n := network.NewElectricalNetwork()
	w := ecs.NewWorld()
	m := ecs.NewMap1[struct{}](w)
	e1 := m.NewEntity(&struct{}{})
	e2 := m.NewEntity(&struct{}{})
	e3 := m.NewEntity(&struct{}{})

	b1 := mustAddBus(t, n, e1, network.BusGenerator)
	b2 := mustAddBus(t, n, e2, network.BusJunction)
	b3 := mustAddBus(t, n, e3, network.BusLoad)

	n.AddBranch(b1.ID, b2.ID, 0, 0)
	n.AddBranch(b2.ID, b3.ID, 0, 0)

	nb1 := n.Neighbors(b1.ID)
	if len(nb1) != 1 || nb1[0].ID != b2.ID {
		t.Fatalf("b1 neighbors = %v, want [b2]", nb1)
	}

	nb2 := n.Neighbors(b2.ID)
	if len(nb2) != 2 {
		t.Fatalf("b2 neighbors = %v, want 2", nb2)
	}
}

func TestRemoveBus(t *testing.T) {
	n := network.NewElectricalNetwork()
	w := ecs.NewWorld()
	m := ecs.NewMap1[struct{}](w)
	e1 := m.NewEntity(&struct{}{})
	e2 := m.NewEntity(&struct{}{})

	b1 := mustAddBus(t, n, e1, network.BusGenerator)
	b2 := mustAddBus(t, n, e2, network.BusLoad)
	n.AddBranch(b1.ID, b2.ID, 0, 0)

	n.RemoveBus(b1.ID)

	if _, ok := n.Bus(b1.ID); ok {
		t.Fatal("bus b1 should be gone")
	}
	if _, ok := n.BusForEntity(e1); ok {
		t.Fatal("entityBus entry for e1 should be gone")
	}
	if len(n.Branches()) != 0 {
		t.Fatal("incident branch should have been removed with b1")
	}
	if nb := n.Neighbors(b2.ID); len(nb) != 0 {
		t.Fatalf("b2 should have no neighbors after b1 removed, got %v", nb)
	}
}

func TestRemoveBranch(t *testing.T) {
	n := network.NewElectricalNetwork()
	w := ecs.NewWorld()
	m := ecs.NewMap1[struct{}](w)
	e1 := m.NewEntity(&struct{}{})
	e2 := m.NewEntity(&struct{}{})

	b1 := mustAddBus(t, n, e1, network.BusGenerator)
	b2 := mustAddBus(t, n, e2, network.BusLoad)
	br := n.AddBranch(b1.ID, b2.ID, 0, 0)

	n.RemoveBranch(br.ID)

	if len(n.Branches()) != 0 {
		t.Fatal("branch should be gone")
	}
	if len(n.Neighbors(b1.ID)) != 0 || len(n.Neighbors(b2.ID)) != 0 {
		t.Fatal("both buses should have empty neighbor lists")
	}
}

func TestDirtyFlag(t *testing.T) {
	n := network.NewElectricalNetwork()
	if n.Dirty {
		t.Fatal("new network should not be dirty")
	}

	w := ecs.NewWorld()
	e := ecs.NewMap1[struct{}](w).NewEntity(&struct{}{})
	b := mustAddBus(t, n, e, network.BusGenerator)
	if !n.Dirty {
		t.Fatal("AddBus should mark dirty")
	}

	n.ClearDirty()
	n.SetBusSpec(b.ID, network.SlackSpec(230, 0))
	if !n.Dirty {
		t.Fatal("SetBusSpec should mark dirty")
	}

	n.ClearDirty()
	n.RemoveBus(b.ID)
	if !n.Dirty {
		t.Fatal("RemoveBus should mark dirty")
	}
}
