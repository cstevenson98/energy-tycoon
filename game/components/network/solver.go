package network

import (
	"fmt"
	"math"

	"github.com/cstevenson98/energy-tycoon/pkg/nr"
	"gonum.org/v1/gonum/mat"
)

// Solver is the interface for network analysis algorithms. Implementations
// read BusSpec boundary conditions from net.State and write BusResult /
// BranchResult solved quantities back into the same StaticState.
type Solver interface {
	Solve(net *ElectricalNetwork) error
}

// LoadflowSolver performs an AC Newton-Raphson power flow.
//
// On each call to Solve:
//  1. The Y-bus (G + jB) is built as a SparseMatrix from current topology.
//  2. The Jacobian's sparsity structure is derived from the same topology and
//     pre-allocated as a SparseMatrix once — only values change inside the loop.
//  3. newton-Raphson iterates until ‖f‖₂ < Tol or MaxIter is exceeded.
//  4. Results are written back to net.State.
//
// Sign convention: P_spec > 0 means net generation (generator convention).
// Load buses must have P_spec < 0. For a consumer-convention load of P_kW,
// set the bus spec as PQSpec(−P_kW*1000, −Q_kVAR*1000) (plain watts/VAR —
// see the package doc in ybus.go for why no per-unit base is needed).
type LoadflowSolver struct {
	MaxIter int     // default 50
	Tol     float64 // convergence criterion on ‖f‖₂ (default 1e-6)
}

// NewLoadflowSolver creates a LoadflowSolver with default parameters.
func NewLoadflowSolver() *LoadflowSolver { return &LoadflowSolver{} }

const (
	defaultLFMaxIter = 50
	defaultLFTol     = 1e-6
)

// Solve runs the AC power flow on net and writes results to net.State.
// Returns a non-nil error if the solver did not converge or the network has
// no slack bus; always writes the best available result even on error.
// LastError mirrors the returned error (empty on success) for ImGui.
func (s *LoadflowSolver) Solve(net *ElectricalNetwork) error {
	state := net.State
	if state == nil {
		err := fmt.Errorf("loadflow: network has no StaticState")
		return err
	}
	state.Converged = false
	state.Iterations = 0
	state.LastError = ""

	if len(net.Buses()) == 0 {
		state.Converged = true
		return nil
	}

	hasSlack := false
	for _, bs := range state.Buses {
		if bs.Spec.Formulation == Slack {
			hasSlack = true
			break
		}
	}
	if !hasSlack {
		state.LastError = "loadflow: no slack bus in network"
		return fmt.Errorf("%s", state.LastError)
	}

	yb := BuildYBus(net)
	bo := yb.BO

	if bo.stateSize == 0 {
		state.Converged = true
		writeAllResults(net, yb, nil)
		return nil
	}

	maxIter := s.MaxIter
	if maxIter == 0 {
		maxIter = defaultLFMaxIter
	}
	tol := s.Tol
	if tol == 0 {
		tol = defaultLFTol
	}

	jacTemplate, jacUpdates := buildJacobianTemplate(state, bo, net)
	nrSolver := &nr.NewtonRaphson{
		MaxIter:     maxIter,
		Tol:         tol,
		LinearSolve: nr.SuperLUSolver(),
	}
	res, err := nrSolver.Solve(
		buildX0(state, bo),
		residualFunc(state, yb),
		jacobianFunc(state, yb, jacTemplate, jacUpdates),
	)

	state.Converged = res.Converged
	state.Iterations = res.Iterations
	writeAllResults(net, yb, res.X)

	if err != nil {
		state.LastError = err.Error()
		return fmt.Errorf("loadflow: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Jacobian sparsity pre-computation
// ---------------------------------------------------------------------------

// jacNZ describes one non-zero entry in the Jacobian and the information
// needed to compute its value at each NR iteration.
type jacNZ struct {
	jRow, jCol int  // position in the state-sized Jacobian
	bi, bj     int  // Y-bus row indices of the equation bus (i) and variable bus (j)
	isDiag     bool // true when bi == bj (different formula for diagonal)
	// kind encodes which sub-block this entry belongs to:
	//   0 = −∂P/∂δ  (H block)
	//   1 = −∂P/∂|V| (N block)
	//   2 = −∂Q/∂δ  (J block)
	//   3 = −∂Q/∂|V| (L block)
	kind int8
}

// buildJacobianTemplate constructs the sparse Jacobian template and returns
// it together with the ordered list of non-zero entries with all information
// needed to fill values from (Vm, Va, P_calc, Q_calc) at each iteration.
//
// The Jacobian's sparsity pattern mirrors the Y-bus pattern restricted to the
// free-variable sub-blocks:
//
//	Non-zero at (equation i, variable j) iff Y_ij ≠ 0 (structurally).
func buildJacobianTemplate(state *StaticState, bo *busOrdering, net *ElectricalNetwork) (*SparseMatrix, []jacNZ) {
	// Enumerate all (bi, bj) pairs where Y_ij is a structural non-zero.
	type pair struct{ bi, bj int }
	pairs := make([]pair, 0, bo.n+2*len(net.Branches()))
	for i := 0; i < bo.n; i++ {
		pairs = append(pairs, pair{i, i}) // diagonal always present
	}
	for _, br := range net.Branches() {
		i, okI := bo.rowOf[br.From]
		j, okJ := bo.rowOf[br.To]
		if !okI || !okJ {
			continue
		}
		pairs = append(pairs, pair{i, j})
		pairs = append(pairs, pair{j, i})
	}

	pattern := make([][2]int, 0, 4*len(pairs))
	updates := make([]jacNZ, 0, 4*len(pairs))

	for _, p := range pairs {
		bi, bj := p.bi, p.bj
		idI := bo.allIDs[bi]
		idJ := bo.allIDs[bj]
		ai := bo.angleIdx[idI] // row for ΔP_i equation (-1 if slack)
		vi := bo.voltIdx[idI]  // offset row for ΔQ_i equation (-1 if not PQ)
		aj := bo.angleIdx[idJ] // col for δ_j variable (-1 if slack)
		vj := bo.voltIdx[idJ]  // offset col for |V_j| variable (-1 if not PQ)
		diag := bi == bj

		// H block: equation=ΔP_i, variable=δ_j
		if ai >= 0 && aj >= 0 {
			r, c := ai, aj
			pattern = append(pattern, [2]int{r, c})
			updates = append(updates, jacNZ{r, c, bi, bj, diag, 0})
		}
		// N block: equation=ΔP_i, variable=|V_j|
		if ai >= 0 && vj >= 0 {
			r, c := ai, bo.nAngle+vj
			pattern = append(pattern, [2]int{r, c})
			updates = append(updates, jacNZ{r, c, bi, bj, diag, 1})
		}
		// J block: equation=ΔQ_i, variable=δ_j
		if vi >= 0 && aj >= 0 {
			r, c := bo.nAngle+vi, aj
			pattern = append(pattern, [2]int{r, c})
			updates = append(updates, jacNZ{r, c, bi, bj, diag, 2})
		}
		// L block: equation=ΔQ_i, variable=|V_j|
		if vi >= 0 && vj >= 0 {
			r, c := bo.nAngle+vi, bo.nAngle+vj
			pattern = append(pattern, [2]int{r, c})
			updates = append(updates, jacNZ{r, c, bi, bj, diag, 3})
		}
	}

	return NewSparseFromPattern(bo.stateSize, bo.stateSize, pattern), updates
}

// ---------------------------------------------------------------------------
// NR closures
// ---------------------------------------------------------------------------

// residualFunc returns the NR residual closure f(x) = [ΔP | ΔQ].
//
//	ΔP[i] = P_spec[i] − P_calc(Vm,Va)[i]   for non-slack bus i
//	ΔQ[i] = Q_spec[i] − Q_calc(Vm,Va)[i]   for PQ bus i
func residualFunc(state *StaticState, yb *YBus) nr.ResidualFunc {
	bo := yb.BO
	return func(x *mat.VecDense) *mat.VecDense {
		Vm, Va := ExtractVmVa(x, state, bo)
		res := mat.NewVecDense(bo.stateSize, nil)
		for _, id := range bo.allIDs {
			bs := state.Buses[id]
			i := bo.rowOf[id]
			Pcalc, Qcalc := CalcPQ(i, Vm, Va, yb)

			if ai := bo.angleIdx[id]; ai >= 0 {
				res.SetVec(ai, bs.Spec.PInject-Pcalc)
			}
			if vi := bo.voltIdx[id]; vi >= 0 {
				res.SetVec(bo.nAngle+vi, bs.Spec.QInject-Qcalc)
			}
		}
		return res
	}
}

// jacobianFunc returns the NR Jacobian closure J(x) = ∂f/∂x.
//
// The Jacobian is stored in the pre-allocated sparseJ SparseMatrix whose
// structure never changes. On every call the values are zeroed and then each
// jacNZ entry in updates is computed and written using only the specific
// formula for that entry's sub-block and diagonal/off-diagonal type.
//
// Because f = P_spec − P_calc, all values are negated vs the standard power-
// flow Jacobian (∂P_calc/∂x).
//
// Diagonal formulas (i == j):
//
//	−∂P/∂δ_i   = +Q_i + B_ii·Vm_i²
//	−∂P/∂|V_i| = −(P_i + G_ii·Vm_i²) / Vm_i
//	−∂Q/∂δ_i   = −P_i + G_ii·Vm_i²
//	−∂Q/∂|V_i| = −(Q_i − B_ii·Vm_i²) / Vm_i
//
// Off-diagonal formulas (i ≠ j):
//
//	−∂P/∂δ_j   = −Vm_i Vm_j (G_ij sin θ_ij − B_ij cos θ_ij)
//	−∂P/∂|V_j| = −Vm_i      (G_ij cos θ_ij + B_ij sin θ_ij)
//	−∂Q/∂δ_j   =  Vm_i Vm_j (G_ij cos θ_ij + B_ij sin θ_ij)
//	−∂Q/∂|V_j| = −Vm_i      (G_ij sin θ_ij − B_ij cos θ_ij)
func jacobianFunc(state *StaticState, yb *YBus, sparseJ *SparseMatrix, updates []jacNZ) nr.JacobianFunc {
	bo := yb.BO
	return func(x *mat.VecDense) mat.Matrix {
		Vm, Va := ExtractVmVa(x, state, bo)

		// Pre-compute total P and Q at every bus (needed for diagonal terms).
		Pcalc := make([]float64, bo.n)
		Qcalc := make([]float64, bo.n)
		for k := 0; k < bo.n; k++ {
			Pcalc[k], Qcalc[k] = CalcPQ(k, Vm, Va, yb)
		}

		// Reset stored values; then write only the non-zero entries.
		sparseJ.Zero()

		for _, u := range updates {
			bi, bj := u.bi, u.bj
			var val float64

			if u.isDiag {
				vi2 := Vm[bi] * Vm[bi]
				gii := yb.G.At(bi, bi)
				bii := yb.B.At(bi, bi)
				switch u.kind {
				case 0: // −∂P/∂δ diagonal = +Q + B·V²
					val = Qcalc[bi] + bii*vi2
				case 1: // −∂P/∂|V| diagonal = −(P + G·V²)/V
					if Vm[bi] != 0 {
						val = -(Pcalc[bi] + gii*vi2) / Vm[bi]
					}
				case 2: // −∂Q/∂δ diagonal = −P + G·V²
					val = -Pcalc[bi] + gii*vi2
				case 3: // −∂Q/∂|V| diagonal = −(Q − B·V²)/V
					if Vm[bi] != 0 {
						val = -(Qcalc[bi] - bii*vi2) / Vm[bi]
					}
				}
			} else {
				θij := Va[bi] - Va[bj]
				gij := yb.G.At(bi, bj)
				bij := yb.B.At(bi, bj)
				cosθ := math.Cos(θij)
				sinθ := math.Sin(θij)
				switch u.kind {
				case 0: // −∂P/∂δ off-diag
					val = -Vm[bi] * Vm[bj] * (gij*sinθ - bij*cosθ)
				case 1: // −∂P/∂|V| off-diag
					val = -Vm[bi] * (gij*cosθ + bij*sinθ)
				case 2: // −∂Q/∂δ off-diag
					val = Vm[bi] * Vm[bj] * (gij*cosθ + bij*sinθ)
				case 3: // −∂Q/∂|V| off-diag
					val = -Vm[bi] * (gij*sinθ - bij*cosθ)
				}
			}

			sparseJ.Set(u.jRow, u.jCol, val)
		}

		return sparseJ
	}
}

// ---------------------------------------------------------------------------
// Result writeback
// ---------------------------------------------------------------------------

// buildX0 assembles the initial NR state vector from BusSpec values.
func buildX0(state *StaticState, bo *busOrdering) *mat.VecDense {
	// Flat-start fallback for PQ voltage magnitudes: use the network's own
	// slack-bus voltage as the "nominal" scale, rather than a hardcoded
	// constant. This keeps the solver unit-agnostic — it works the same
	// whether buses are specified in per-unit (~1.0) or real volts (~230).
	flatV := 1.0
	for _, bs := range state.Buses {
		if bs.Spec.Formulation == Slack && bs.Spec.VoltMag != 0 {
			flatV = bs.Spec.VoltMag
			break
		}
	}

	x0 := mat.NewVecDense(bo.stateSize, nil)
	for _, id := range bo.allIDs {
		bs := state.Buses[id]
		if ai := bo.angleIdx[id]; ai >= 0 {
			x0.SetVec(ai, bs.Spec.VoltAng)
		}
		if vi := bo.voltIdx[id]; vi >= 0 {
			v := bs.Spec.VoltMag
			if v == 0 {
				v = flatV
			}
			x0.SetVec(bo.nAngle+vi, v)
		}
	}
	return x0
}

// writeAllResults decodes the solved state vector x and writes BusResult and
// BranchResult entries into net.State. x may be nil for all-slack networks.
func writeAllResults(net *ElectricalNetwork, yb *YBus, x *mat.VecDense) {
	state := net.State
	bo := yb.BO

	Vm, Va := ExtractVmVa(x, state, bo)

	// Bus results
	for _, id := range bo.allIDs {
		i := bo.rowOf[id]
		Pcalc, Qcalc := CalcPQ(i, Vm, Va, yb)
		state.Buses[id].Result = BusResult{
			VoltMag: Vm[i],
			VoltAng: Va[i],
			PInject: Pcalc,
			QInject: Qcalc,
		}
	}

	// Branch results from series y = g + jb = 1/(r+jx):
	//   I_ij = y · (V_i − V_j);  S_from = V_i · conj(I_ij)
	for id, br := range net.Branches() {
		i, okI := bo.rowOf[br.From]
		j, okJ := bo.rowOf[br.To]
		if !okI || !okJ {
			continue
		}
		vi, vj := Vm[i], Vm[j]
		vai, vaj := Va[i], Va[j]
		g, b := seriesGB(br.Resistance, br.Reactance)

		viRe, viIm := vi*math.Cos(vai), vi*math.Sin(vai)
		vjRe, vjIm := vj*math.Cos(vaj), vj*math.Sin(vaj)
		dvr, dvIm := viRe-vjRe, viIm-vjIm
		iRe := g*dvr - b*dvIm
		iIm := b*dvr + g*dvIm

		state.Branches[id].Result = BranchResult{
			CurrentMag: math.Hypot(iRe, iIm),
			PFrom:      viRe*iRe + viIm*iIm,
			PTo:        -(vjRe*iRe + vjIm*iIm),
			QFrom:      viIm*iRe - viRe*iIm,
			QTo:        -(vjIm*iRe - vjRe*iIm),
		}
	}
}
