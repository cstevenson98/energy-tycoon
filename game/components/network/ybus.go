// Package network — Y-bus construction and bus ordering for AC power flow.
//
// Convention used throughout this file and solver.go:
//
//	P_calc[i] = Re(S[i])  where  S = V ⊙ conj(Y·V)
//
// This gives P_calc > 0 when the bus is a net *generator* (power injected into
// the network from an external source). For a load bus P_calc < 0. This is the
// standard generator convention used in classical power-flow literature.
//
// BusSpec.PInject follows the same sign: positive = generation.
// If you have a load's consumed power P_kW (consumer convention, positive),
// set the bus spec as PQSpec(−P_kW*1000, −Q_kVAR*1000) — everything here is
// plain SI units (volts, ohms, watts, VAR), so no base-MVA normalisation is
// needed; see NominalVoltageV in state.go.
package network

import (
	"math"
	"sort"

	"gonum.org/v1/gonum/mat"
)

// minResistance is the floor applied to any branch resistance to prevent
// infinite (and numerically extreme) admittance. Direct connections (R=0) —
// e.g. a house bus tied to a line-segment bus — use this value.
//
// Must stay well below one line-cell R (~0.0016 Ω for 10 m of 185 mm² Al) so
// contact links remain near-shorts, but not so small that G = 1/R blows up
// the SI Jacobian (1e-6 Ω → G ~ 1e6 S stalled NR previously).
const minResistance = 1e-4 // Ω

// busOrdering defines a deterministic mapping from BusIDs to:
//
//   - row / column indices in the n×n Y-bus matrices
//   - positions in the Newton-Raphson state vector
//
// State vector layout:  x = [ δ for non-slack buses | |V| for PQ buses ]
//
// Slack buses contribute no equations. PV buses contribute only ΔP. PQ buses
// contribute both ΔP and ΔQ.
type busOrdering struct {
	n      int
	allIDs []BusID // allIDs[k] = bus with Y-bus row k (sorted by BusID)

	// Y-bus matrix row/column for each bus
	rowOf map[BusID]int

	// State-vector angle sub-block: index of δ_i in x[0 : nAngle]
	// -1 for Slack buses (angle is fixed, no equation).
	nAngle   int
	angleIdx map[BusID]int

	// State-vector voltage sub-block: index of |V|_i in x[nAngle : nAngle+nVolt]
	// -1 for Slack and PV buses (magnitude is fixed or not free).
	nVolt   int
	voltIdx map[BusID]int

	stateSize int // = nAngle + nVolt
}

// newBusOrdering builds the index maps for the given bus IDs (already sorted).
func newBusOrdering(net *ElectricalNetwork, ids []BusID) *busOrdering {
	bo := &busOrdering{
		n:        len(ids),
		allIDs:   ids,
		rowOf:    make(map[BusID]int, len(ids)),
		angleIdx: make(map[BusID]int, len(ids)),
		voltIdx:  make(map[BusID]int, len(ids)),
	}
	for i, id := range ids {
		bo.rowOf[id] = i
		bo.angleIdx[id] = -1
		bo.voltIdx[id] = -1
	}

	ai := 0
	for _, id := range ids {
		if net.State.Buses[id].Spec.Formulation != Slack {
			bo.angleIdx[id] = ai
			ai++
		}
	}
	bo.nAngle = ai

	vi := 0
	for _, id := range ids {
		if net.State.Buses[id].Spec.Formulation == PQ {
			bo.voltIdx[id] = vi
			vi++
		}
	}
	bo.nVolt = vi
	bo.stateSize = bo.nAngle + bo.nVolt

	return bo
}

// sortedBusIDs returns all bus IDs in net, sorted ascending.
func sortedBusIDs(net *ElectricalNetwork) []BusID {
	buses := net.Buses()
	ids := make([]BusID, 0, len(buses))
	for id := range buses {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// YBus holds the nodal admittance matrix split into its real part G and
// imaginary part B (Y = G + jB), plus the index mapping used to build it.
//
// Both matrices are stored as SparseMatrix with the Y-bus sparsity pattern
// (n diagonal entries + 2 entries per included branch).
type YBus struct {
	G  *SparseMatrix // conductance (real part of Y)
	B  *SparseMatrix // susceptance (imaginary part of Y)
	BO *busOrdering
}

// YBusPattern returns the (row, col) pairs for the structural non-zeros of
// the Y-bus for the buses in bo, using only branches whose both ends are in bo.
func YBusPattern(net *ElectricalNetwork, bo *busOrdering) [][2]int {
	n := bo.n
	pattern := make([][2]int, 0, n+2*len(net.Branches()))
	for i := 0; i < n; i++ {
		pattern = append(pattern, [2]int{i, i})
	}
	for _, br := range net.Branches() {
		i, okI := bo.rowOf[br.From]
		j, okJ := bo.rowOf[br.To]
		if !okI || !okJ {
			continue
		}
		pattern = append(pattern, [2]int{i, j})
		pattern = append(pattern, [2]int{j, i})
	}
	return pattern
}

// seriesGB returns the series conductance and susceptance for impedance r+jx.
// Resistance is floored at minResistance; reactance is used as-is.
func seriesGB(r, x float64) (g, b float64) {
	if r < minResistance {
		r = minResistance
	}
	denom := r*r + x*x
	g = r / denom
	b = -x / denom
	return g, b
}

// BuildYBus constructs the Y-bus from the network's full topology.
func BuildYBus(net *ElectricalNetwork) *YBus {
	return BuildYBusFor(net, sortedBusIDs(net))
}

// BuildYBusFor constructs the Y-bus restricted to the given bus IDs (one
// island). Branches with an endpoint outside ids are ignored.
func BuildYBusFor(net *ElectricalNetwork, ids []BusID) *YBus {
	bo := newBusOrdering(net, ids)
	pattern := YBusPattern(net, bo)

	G := NewSparseFromPattern(bo.n, bo.n, pattern)
	B := NewSparseFromPattern(bo.n, bo.n, pattern)

	for _, br := range net.Branches() {
		i, okI := bo.rowOf[br.From]
		j, okJ := bo.rowOf[br.To]
		if !okI || !okJ {
			continue
		}
		g, b := seriesGB(br.Resistance, br.Reactance)

		G.Add(i, j, -g)
		G.Add(j, i, -g)
		B.Add(i, j, -b)
		B.Add(j, i, -b)

		G.Add(i, i, g)
		G.Add(j, j, g)
		B.Add(i, i, b)
		B.Add(j, j, b)
	}

	return &YBus{G: G, B: B, BO: bo}
}

// CalcPQ computes the net active (P) and reactive (Q) power *injection* at
// the bus with Y-bus row i, given voltage magnitudes Vm and angles Va (rad).
//
//	P[i] = |V_i| · Σ_j |V_j| · (G_ij·cos θ_ij + B_ij·sin θ_ij)
//	Q[i] = |V_i| · Σ_j |V_j| · (G_ij·sin θ_ij − B_ij·cos θ_ij)
//
// Only structural non-zeros in G are iterated (O(degree_i)); B is sampled
// at the same columns (also sparse for the same pattern).
// P > 0 means net generation at bus i (generator convention).
func CalcPQ(i int, Vm, Va []float64, yb *YBus) (P, Q float64) {
	yb.G.ForEachInRow(i, func(j int, gij float64) {
		bij := yb.B.At(i, j)
		θij := Va[i] - Va[j]
		cosθ := math.Cos(θij)
		sinθ := math.Sin(θij)
		P += Vm[j] * (gij*cosθ + bij*sinθ)
		Q += Vm[j] * (gij*sinθ - bij*cosθ)
	})
	P *= Vm[i]
	Q *= Vm[i]
	return
}

// ExtractVmVa reads the voltage magnitudes and angles from the NR state
// vector x, filling in spec-fixed values for Slack (both Vm and Va) and
// PV (only Vm) buses. x may be nil when stateSize == 0 (all-slack network);
// in that case all values are taken from the specs.
func ExtractVmVa(x *mat.VecDense, state *StaticState, bo *busOrdering) (Vm, Va []float64) {
	Vm = make([]float64, bo.n)
	Va = make([]float64, bo.n)
	for _, id := range bo.allIDs {
		i := bo.rowOf[id]
		bs := state.Buses[id]
		if ai := bo.angleIdx[id]; ai >= 0 && x != nil {
			Va[i] = x.AtVec(ai)
		} else {
			Va[i] = bs.Spec.VoltAng
		}
		if vi := bo.voltIdx[id]; vi >= 0 && x != nil {
			Vm[i] = x.AtVec(bo.nAngle + vi)
		} else {
			Vm[i] = bs.Spec.VoltMag
		}
	}
	return
}
