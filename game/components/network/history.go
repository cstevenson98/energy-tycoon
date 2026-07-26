package network

import "github.com/cstevenson98/gowasm-engine/pkg/ecs"

// DefaultHistoryCap is the number of past solves retained per series
// (bus P/Q/V/δ and branch |I|).
const DefaultHistoryCap = 25

// Series is a fixed-capacity ring buffer of float64 samples, oldest → newest.
// Once full, Add overwrites the oldest entry. Zero value is not usable;
// construct with NewSeries.
type Series struct {
	buf  []float64
	head int // index of the oldest sample when len == cap
	n    int // number of valid samples
}

// NewSeries creates an empty series with the given capacity. If cap < 1,
// DefaultHistoryCap is used.
func NewSeries(cap int) Series {
	if cap < 1 {
		cap = DefaultHistoryCap
	}
	return Series{buf: make([]float64, cap)}
}

// Cap returns the maximum number of samples the series can hold.
func (s *Series) Cap() int { return len(s.buf) }

// Len returns the number of samples currently stored.
func (s *Series) Len() int { return s.n }

// Add appends v, dropping the oldest sample if the series is full.
func (s *Series) Add(v float64) {
	if len(s.buf) == 0 {
		*s = NewSeries(DefaultHistoryCap)
	}
	if s.n < len(s.buf) {
		s.buf[(s.head+s.n)%len(s.buf)] = v
		s.n++
		return
	}
	s.buf[s.head] = v
	s.head = (s.head + 1) % len(s.buf)
}

// Clear removes all samples.
func (s *Series) Clear() {
	s.head = 0
	s.n = 0
}

// Values returns a copy of the samples in chronological order (oldest first).
func (s *Series) Values() []float64 {
	out := make([]float64, s.n)
	for i := 0; i < s.n; i++ {
		out[i] = s.buf[(s.head+i)%len(s.buf)]
	}
	return out
}

// Last returns the most recently added sample, or false if empty.
func (s *Series) Last() (float64, bool) {
	if s.n == 0 {
		return 0, false
	}
	i := (s.head + s.n - 1) % len(s.buf)
	return s.buf[i], true
}

// BusHistory is an ECS component on grid entities that participate in the
// electrical network. It stores the last DefaultHistoryCap solve results for
// active/reactive injection (W, VAR), voltage magnitude (V), and angle (rad).
type BusHistory struct {
	P     Series // active power injection, W (+gen)
	Q     Series // reactive power injection, VAR
	V     Series // |V|, volts
	Delta Series // voltage angle, radians
}

// NewBusHistory creates empty P/Q/V/δ series with DefaultHistoryCap.
func NewBusHistory() BusHistory {
	return BusHistory{
		P:     NewSeries(DefaultHistoryCap),
		Q:     NewSeries(DefaultHistoryCap),
		V:     NewSeries(DefaultHistoryCap),
		Delta: NewSeries(DefaultHistoryCap),
	}
}

// Add appends one solve sample to every bus series.
func (h *BusHistory) Add(p, q, v, delta float64) {
	h.P.Add(p)
	h.Q.Add(q)
	h.V.Add(v)
	h.Delta.Add(delta)
}

// Clear empties all bus series.
func (h *BusHistory) Clear() {
	h.P.Clear()
	h.Q.Clear()
	h.V.Clear()
	h.Delta.Clear()
}

// BranchHistory stores the last DefaultHistoryCap solve results for branch
// current magnitude (A). Branches are not grid entities, so this lives on
// BranchState inside ElectricalNetwork.State rather than as an ECS component.
type BranchHistory struct {
	Current Series // |I|, amps
}

// NewBranchHistory creates an empty |I| series with DefaultHistoryCap.
func NewBranchHistory() BranchHistory {
	return BranchHistory{Current: NewSeries(DefaultHistoryCap)}
}

// Add appends one |I| sample.
func (h *BranchHistory) Add(current float64) {
	h.Current.Add(current)
}

// Clear empties the current series.
func (h *BranchHistory) Clear() {
	h.Current.Clear()
}

// RecordHistory reads the latest BusResult / BranchResult from net.State and
// appends one sample to each linked entity's BusHistory and each branch's
// BranchHistory. Entities without a BusHistory component are skipped.
// Call after a solve that has written results into net.State.
func RecordHistory(w *ecs.World, net *ElectricalNetwork) {
	if net == nil || net.State == nil || w == nil {
		return
	}

	busHist := ecs.NewMap1[BusHistory](w)
	for id, bus := range net.Buses() {
		bs, ok := net.State.Buses[id]
		if !ok || bs == nil {
			continue
		}
		h := busHist.Get(bus.Entity)
		if h == nil {
			continue
		}
		h.Add(bs.Result.PInject, bs.Result.QInject, bs.Result.VoltMag, bs.Result.VoltAng)
	}

	for _, brState := range net.State.Branches {
		if brState == nil {
			continue
		}
		brState.History.Add(brState.Result.CurrentMag)
	}
}

// ClearAllHistory clears BusHistory on every networked entity and BranchHistory
// on every branch in net.State.
func ClearAllHistory(w *ecs.World, net *ElectricalNetwork) {
	if net == nil || net.State == nil || w == nil {
		return
	}
	busHist := ecs.NewMap1[BusHistory](w)
	for _, bus := range net.Buses() {
		if h := busHist.Get(bus.Entity); h != nil {
			h.Clear()
		}
	}
	for _, brState := range net.State.Branches {
		if brState != nil {
			brState.History.Clear()
		}
	}
}
