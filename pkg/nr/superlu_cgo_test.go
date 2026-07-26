//go:build cgo

package nr_test

import (
	"math"
	"testing"

	"github.com/cstevenson98/energy-tycoon/pkg/nr"
	"gonum.org/v1/gonum/mat"
)

// TestSuperLUSolverLinear solves the same 2x2 system as TestLinearSystem,
// but calls SuperLUSolver directly (bypassing NewtonRaphson) so we can check
// the raw linear-solve result.
func TestSuperLUSolverLinear(t *testing.T) {
	// 2x + y = 5
	// x + 3y = 7  -> x=8/5, y=9/5
	A := mat.NewDense(2, 2, []float64{2, 1, 1, 3})
	b := mat.NewVecDense(2, []float64{5, 7})

	x, err := nr.SuperLUSolver()(A, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(x.AtVec(0)-8.0/5.0) > 1e-9 {
		t.Errorf("x[0] = %v, want %v", x.AtVec(0), 8.0/5.0)
	}
	if math.Abs(x.AtVec(1)-9.0/5.0) > 1e-9 {
		t.Errorf("x[1] = %v, want %v", x.AtVec(1), 9.0/5.0)
	}
}

// TestSuperLUSolverNonlinearViaNR re-runs the classic 2D Newton-Raphson test
// (unit circle intersect diagonal) with SuperLUSolver wired in as the linear
// solve.
func TestSuperLUSolverNonlinearViaNR(t *testing.T) {
	f := func(x *mat.VecDense) *mat.VecDense {
		x0, x1 := x.AtVec(0), x.AtVec(1)
		return mat.NewVecDense(2, []float64{
			x0*x0 + x1*x1 - 1,
			x0 - x1,
		})
	}
	jac := func(x *mat.VecDense) mat.Matrix {
		x0, x1 := x.AtVec(0), x.AtVec(1)
		return mat.NewDense(2, 2, []float64{
			2 * x0, 2 * x1,
			1, -1,
		})
	}

	solver := &nr.NewtonRaphson{MaxIter: 50, Tol: 1e-12, LinearSolve: nr.SuperLUSolver()}
	x0 := mat.NewVecDense(2, []float64{0.6, 0.8})
	res, err := solver.Solve(x0, f, jac)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Converged {
		t.Fatalf("expected convergence, residual = %e", res.Residual)
	}
	want := 1.0 / math.Sqrt2
	if math.Abs(res.X.AtVec(0)-want) > 1e-10 {
		t.Errorf("x[0] = %v, want %v", res.X.AtVec(0), want)
	}
	if math.Abs(res.X.AtVec(1)-want) > 1e-10 {
		t.Errorf("x[1] = %v, want %v", res.X.AtVec(1), want)
	}
}

// denseAsNonZeroer wraps a *mat.Dense and implements nr.NonZeroer, so tests
// can exercise SuperLUSolver's O(nnz) toCSC path without importing the
// network package's SparseMatrix (which would add a test-only dependency
// edge back onto a package that already depends on nr).
type denseAsNonZeroer struct{ *mat.Dense }

func (d denseAsNonZeroer) ForEachNonZero(f func(i, j int, v float64)) {
	r, c := d.Dims()
	for i := 0; i < r; i++ {
		for j := 0; j < c; j++ {
			if v := d.At(i, j); v != 0 {
				f(i, j, v)
			}
		}
	}
}

// TestSuperLUSolverSparseNonZeroerPath checks that the NonZeroer fast path
// in toCSC produces the same result as the plain mat.Matrix fallback, for a
// larger, genuinely sparse (tridiagonal) system.
func TestSuperLUSolverSparseNonZeroerPath(t *testing.T) {
	const n = 6
	dense := mat.NewDense(n, n, nil)
	for i := 0; i < n; i++ {
		dense.Set(i, i, 4)
		if i > 0 {
			dense.Set(i, i-1, -1)
		}
		if i < n-1 {
			dense.Set(i, i+1, -1)
		}
	}
	b := mat.NewVecDense(n, nil)
	for i := 0; i < n; i++ {
		b.SetVec(i, float64(i+1))
	}

	xFallback, err := nr.SuperLUSolver()(dense, b)
	if err != nil {
		t.Fatalf("dense fallback path: unexpected error: %v", err)
	}

	xSparse, err := nr.SuperLUSolver()(denseAsNonZeroer{dense}, b)
	if err != nil {
		t.Fatalf("NonZeroer path: unexpected error: %v", err)
	}

	for i := 0; i < n; i++ {
		if math.Abs(xFallback.AtVec(i)-xSparse.AtVec(i)) > 1e-9 {
			t.Errorf("x[%d]: fallback=%v sparse=%v, want equal", i, xFallback.AtVec(i), xSparse.AtVec(i))
		}
	}
}

// TestSuperLUSolverSingularReturnsError ensures a singular matrix produces
// a Go error instead of a panic or silently wrong result.
func TestSuperLUSolverSingularReturnsError(t *testing.T) {
	// Row 1 is a multiple of row 0 -> singular.
	A := mat.NewDense(2, 2, []float64{1, 2, 2, 4})
	b := mat.NewVecDense(2, []float64{1, 2})

	_, err := nr.SuperLUSolver()(A, b)
	if err == nil {
		t.Fatal("expected an error for a singular matrix, got nil")
	}
	t.Logf("got expected error: %v", err)
}
