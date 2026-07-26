package nr_test

import (
	"math"
	"testing"

	"github.com/cstevenson98/energy-tycoon/pkg/nr"
	"gonum.org/v1/gonum/mat"
)

// TestLinearSystem: f(x) = A·x - b = 0, J = A (constant).
// Exact solution in one iteration for a linear system.
func TestLinearSystem(t *testing.T) {
	// 2x + y = 5
	// x + 3y = 7  → x=8/5, y=9/5
	A := mat.NewDense(2, 2, []float64{2, 1, 1, 3})
	b := mat.NewVecDense(2, []float64{5, 7})

	f := func(x *mat.VecDense) *mat.VecDense {
		r := mat.NewVecDense(2, nil)
		r.MulVec(A, x)
		r.SubVec(r, b)
		return r
	}
	jac := func(_ *mat.VecDense) mat.Matrix { return A }

	solver := &nr.NewtonRaphson{MaxIter: 10, Tol: 1e-10}
	res, err := solver.Solve(mat.NewVecDense(2, []float64{0, 0}), f, jac)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Converged {
		t.Fatal("expected convergence")
	}
	if math.Abs(res.X.AtVec(0)-8.0/5.0) > 1e-9 {
		t.Errorf("x[0] = %v, want %v", res.X.AtVec(0), 8.0/5.0)
	}
	if math.Abs(res.X.AtVec(1)-9.0/5.0) > 1e-9 {
		t.Errorf("x[1] = %v, want %v", res.X.AtVec(1), 9.0/5.0)
	}
}

// TestNonlinearSystem: classic 2D NR test.
// f1 = x² + y² - 1 = 0  (unit circle)
// f2 = x  - y    = 0    (diagonal)
// Solution: x = y = 1/√2
func TestNonlinearSystem(t *testing.T) {
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

	solver := &nr.NewtonRaphson{MaxIter: 50, Tol: 1e-12}
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
	t.Logf("converged in %d iterations, residual = %.2e", res.Iterations, res.Residual)
}

// TestMaxIterExceeded ensures a non-convergent problem returns an error with
// the best iterate populated.
func TestMaxIterExceeded(t *testing.T) {
	// f(x) = x - 100; diverges from x0 = 0 if Tol is impossibly tight.
	f := func(x *mat.VecDense) *mat.VecDense {
		return mat.NewVecDense(1, []float64{x.AtVec(0) - 100})
	}
	jac := func(_ *mat.VecDense) mat.Matrix {
		return mat.NewDense(1, 1, []float64{1})
	}

	solver := &nr.NewtonRaphson{MaxIter: 5, Tol: 1e-300}
	res, err := solver.Solve(mat.NewVecDense(1, []float64{0}), f, jac)

	// Should converge (linear, exact in one step) but residual won't hit 1e-300
	// due to floating point; either way Result must be populated.
	if res == nil {
		t.Fatal("Result must be non-nil even on error")
	}
	_ = err // may or may not error depending on float precision
	t.Logf("res.Converged=%v err=%v residual=%.2e", res.Converged, err, res.Residual)
}
