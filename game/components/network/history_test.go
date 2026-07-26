package network_test

import (
	"testing"

	"github.com/cstevenson98/energy-tycoon/game/components/network"
	"github.com/cstevenson98/energy-tycoon/game/components/sim"
	"github.com/cstevenson98/milo/pkg/ecs"
)

func TestSeriesRingBuffer(t *testing.T) {
	s := network.NewSeries(3)
	if s.Len() != 0 {
		t.Fatalf("Len = %d, want 0", s.Len())
	}

	s.Add(1)
	s.Add(2)
	s.Add(3)
	got := s.Values()
	want := []float64{1, 2, 3}
	if !floatSliceEq(got, want) {
		t.Fatalf("Values = %v, want %v", got, want)
	}

	s.Add(4) // drops 1
	got = s.Values()
	want = []float64{2, 3, 4}
	if !floatSliceEq(got, want) {
		t.Fatalf("after overflow Values = %v, want %v", got, want)
	}

	last, ok := s.Last()
	if !ok || last != 4 {
		t.Fatalf("Last = %v %v, want 4 true", last, ok)
	}

	s.Clear()
	if s.Len() != 0 {
		t.Fatalf("after Clear Len = %d, want 0", s.Len())
	}
	if _, ok := s.Last(); ok {
		t.Fatal("Last should be false after Clear")
	}
}

func TestRecordHistory(t *testing.T) {
	net, w := emptyNet()
	e0, e1 := newEntity(w), newEntity(w)

	b0 := mustAddBus(t, net, e0, network.BusGenerator)
	b1 := mustAddBus(t, net, e1, network.BusLoad)
	h0 := network.NewBusHistory()
	h1 := network.NewBusHistory()
	ecs.NewMap1[network.BusHistory](w).Add(e0, &h0)
	ecs.NewMap1[network.BusHistory](w).Add(e1, &h1)

	// 15:30 → 15.5 hours since epoch
	clock := &sim.SimClock{NowMs: sim.EpochMs + 15*sim.MsPerHour + 30*sim.MsPerMinute, Playing: true}
	ecs.SetResource(w, clock)

	net.SetBusSpec(b1.ID, network.PQSpec(-15000, 0))
	net.AddBranch(b0.ID, b1.ID, 0.00164, 0) // one 10 m LV feeder cell

	if err := network.NewLoadflowSolver().Solve(net); err != nil {
		t.Fatalf("solve: %v", err)
	}
	network.RecordHistory(w, net)

	bh0 := ecs.NewMap1[network.BusHistory](w).Get(e0)
	bh1 := ecs.NewMap1[network.BusHistory](w).Get(e1)
	if bh0.V.Len() != 1 || bh1.V.Len() != 1 || bh1.T.Len() != 1 {
		t.Fatalf("bus history len: gen=%d load=%d T=%d, want 1", bh0.V.Len(), bh1.V.Len(), bh1.T.Len())
	}
	th, _ := bh1.T.Last()
	if th != 15.5 {
		t.Errorf("T last = %v, want 15.5 hours since epoch", th)
	}
	v0, _ := bh0.V.Last()
	if v0 < 200 {
		t.Errorf("gen |V| history last = %.2f, want ~230", v0)
	}
	p1, _ := bh1.P.Last()
	if p1 > -10000 {
		t.Errorf("load P history last = %.2f, want ~-15000", p1)
	}

	var brHist *network.BranchHistory
	for _, st := range net.State.Branches {
		brHist = &st.History
		break
	}
	if brHist == nil || brHist.Current.Len() != 1 || brHist.T.Len() != 1 {
		t.Fatal("expected one branch current/time sample")
	}
	i, _ := brHist.Current.Last()
	if i <= 0 {
		t.Errorf("branch |I| = %.4f, want > 0", i)
	}
	bt, _ := brHist.T.Last()
	if bt != 15.5 {
		t.Errorf("branch T = %v, want 15.5", bt)
	}

	// Second solve grows history later (monotonic hours since epoch).
	clock.NowMs = sim.EpochMs + 18*sim.MsPerHour
	net.MarkDirty()
	_ = network.NewLoadflowSolver().Solve(net)
	network.RecordHistory(w, net)
	if bh1.V.Len() != 2 || brHist.Current.Len() != 2 || bh1.T.Len() != 2 {
		t.Fatalf("after 2nd record: bus V len=%d T len=%d branch I len=%d, want 2",
			bh1.V.Len(), bh1.T.Len(), brHist.Current.Len())
	}
	th2, _ := bh1.T.Last()
	if th2 != 18 {
		t.Errorf("T last after 2nd = %v, want 18", th2)
	}

	bh1.Clear()
	if bh1.V.Len() != 0 || bh1.P.Len() != 0 || bh1.T.Len() != 0 {
		t.Fatal("BusHistory.Clear should empty all series")
	}
}

func floatSliceEq(a, b []float64) bool {
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
