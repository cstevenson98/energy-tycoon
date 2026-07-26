//go:build cgo

package nr_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/cstevenson98/energy-tycoon/pkg/nr"
	"gonum.org/v1/gonum/mat"
)

// ---------------------------------------------------------------------------
// Sparse test matrix (NonZeroer) — tridiagonal / banded SPD-ish systems
// ---------------------------------------------------------------------------

// sparseEntries is a minimal CSC/CSR-agnostic sparse matrix for benchmarks:
// pattern + values with ForEachNonZero so SuperLU's toCSC stays O(nnz).
type sparseEntries struct {
	n       int
	entries []struct {
		i, j int
		v    float64
	}
}

func (s *sparseEntries) Dims() (r, c int)         { return s.n, s.n }
func (s *sparseEntries) At(i, j int) float64 {
	for _, e := range s.entries {
		if e.i == i && e.j == j {
			return e.v
		}
	}
	return 0
}
func (s *sparseEntries) T() mat.Matrix { return mat.Transpose{Matrix: s} }

func (s *sparseEntries) ForEachNonZero(f func(i, j int, v float64)) {
	for _, e := range s.entries {
		f(e.i, e.j, e.v)
	}
}

func (s *sparseEntries) nnz() int { return len(s.entries) }

// makeBandSPD builds an n×n symmetric band matrix with half-bandwidth bw
// (diagonal + bw lower/upper). nnz ≈ n*(2*bw+1) - bw*(bw+1).
// Diagonally dominant so LU / SuperLU succeed.
func makeBandSPD(n, bw int) (*sparseEntries, *mat.VecDense) {
	if bw < 0 {
		bw = 0
	}
	if bw >= n {
		bw = n - 1
	}
	s := &sparseEntries{n: n}
	add := func(i, j int, v float64) {
		s.entries = append(s.entries, struct {
			i, j int
			v    float64
		}{i, j, v})
	}
	for i := 0; i < n; i++ {
		diag := float64(2*bw + 2) // diagonally dominant
		add(i, i, diag)
		for d := 1; d <= bw; d++ {
			if i+d < n {
				add(i, i+d, -1)
				add(i+d, i, -1)
			}
		}
	}
	b := mat.NewVecDense(n, nil)
	for i := 0; i < n; i++ {
		b.SetVec(i, 1)
	}
	return s, b
}

func makeTridiag(n int) (*sparseEntries, *mat.VecDense) {
	return makeBandSPD(n, 1)
}

// ---------------------------------------------------------------------------
// Big-O fit helpers: log(t) ≈ intercept + slope * log(x)
// ---------------------------------------------------------------------------

func fitLogLog(xs, ys []float64) (slope, intercept, r2 float64) {
	if len(xs) != len(ys) || len(xs) < 2 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	n := float64(len(xs))
	var sumX, sumY, sumXX, sumYY, sumXY float64
	for i := range xs {
		x := math.Log(xs[i])
		y := math.Log(ys[i])
		sumX += x
		sumY += y
		sumXX += x * x
		sumYY += y * y
		sumXY += x * y
	}
	den := n*sumXX - sumX*sumX
	if den == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	slope = (n*sumXY - sumX*sumY) / den
	intercept = (sumY - slope*sumX) / n

	// R² of the log-log regression.
	meanY := sumY / n
	var ssTot, ssRes float64
	for i := range xs {
		x := math.Log(xs[i])
		y := math.Log(ys[i])
		pred := intercept + slope*x
		ssTot += (y - meanY) * (y - meanY)
		ssRes += (y - pred) * (y - pred)
	}
	if ssTot == 0 {
		r2 = 1
	} else {
		r2 = 1 - ssRes/ssTot
	}
	return slope, intercept, r2
}

func timeSolve(solve nr.LinearSolver, A mat.Matrix, b *mat.VecDense, repeats int) time.Duration {
	// Warmup
	if _, err := solve(A, b); err != nil {
		panic(err)
	}
	start := time.Now()
	for i := 0; i < repeats; i++ {
		if _, err := solve(A, b); err != nil {
			panic(err)
		}
	}
	return time.Since(start) / time.Duration(repeats)
}

func complexityTable(t *testing.T, name string, solve nr.LinearSolver, sizes []int, bw int) {
	t.Helper()
	ns := make([]float64, 0, len(sizes))
	nnzs := make([]float64, 0, len(sizes))
	secs := make([]float64, 0, len(sizes))

	t.Logf("%s  (band half-width=%d)", name, bw)
	t.Logf("%8s %10s %14s %12s %12s", "n", "nnz", "ns/op", "ns/n", "ns/nnz")
	for _, n := range sizes {
		A, b := makeBandSPD(n, bw)
		nnz := A.nnz()
		// More repeats for small n so timing is stable.
		reps := 32
		if n >= 512 {
			reps = 8
		}
		if n >= 2048 {
			reps = 3
		}
		d := timeSolve(solve, A, b, reps)
		nsPer := float64(d.Nanoseconds())
		t.Logf("%8d %10d %14.0f %12.2f %12.2f",
			n, nnz, nsPer, nsPer/float64(n), nsPer/float64(nnz))
		ns = append(ns, float64(n))
		nnzs = append(nnzs, float64(nnz))
		secs = append(secs, nsPer)
	}

	sN, _, rN := fitLogLog(ns, secs)
	sZ, _, rZ := fitLogLog(nnzs, secs)
	t.Logf("fit:  T ~ n^%.2f   (R²=%.4f)   |   T ~ nnz^%.2f   (R²=%.4f)", sN, rN, sZ, rZ)
	t.Logf("hint: sparse tridiag SuperLU often ~ n¹–n¹·⁵")
}

// TestLUComplexityFit reports log-log slopes for SuperLU on tridiagonal
// systems (nnz ≈ 3n). Run with:
//
//	go test ./pkg/nr/ -run ComplexityFit -v
func TestLUComplexityFit(t *testing.T) {
	sizes := []int{64, 128, 256, 512, 1024, 2048}
	complexityTable(t, "SuperLU (tridiag)", nr.SuperLUSolver(), sizes, 1)
}

// TestLUComplexityFitBand grows nnz faster (bw=8).
func TestLUComplexityFitBand(t *testing.T) {
	sizes := []int{64, 128, 256, 512, 1024}
	complexityTable(t, "SuperLU (band bw=8)", nr.SuperLUSolver(), sizes, 8)
}

// ---------------------------------------------------------------------------
// Standard go test -bench targets (ns/op + custom ns/nnz for external fits)
// ---------------------------------------------------------------------------

func BenchmarkSuperLU_Tridiag(b *testing.B) {
	solve := nr.SuperLUSolver()
	for _, n := range []int{50, 100, 200, 400, 800, 1600, 3200} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			A, rhs := makeTridiag(n)
			nnz := A.nnz()
			b.ReportMetric(float64(n), "n")
			b.ReportMetric(float64(nnz), "nnz")
			b.ResetTimer()
			var totalNS int64
			for i := 0; i < b.N; i++ {
				t0 := time.Now()
				if _, err := solve(A, rhs); err != nil {
					b.Fatal(err)
				}
				totalNS += time.Since(t0).Nanoseconds()
			}
			b.ReportMetric(float64(totalNS)/float64(b.N)/float64(nnz), "ns/nnz")
			b.ReportMetric(float64(totalNS)/float64(b.N)/float64(n), "ns/n")
			b.ReportMetric(float64(totalNS)/float64(b.N)/float64(n*n), "ns/n^2")
		})
	}
}

// BenchmarkSuperLU_BandFixedN grows nnz at fixed n (vary bandwidth) so a
// fit vs nnz is not confounded with n.
func BenchmarkSuperLU_BandFixedN(b *testing.B) {
	const n = 800
	solve := nr.SuperLUSolver()
	for _, bw := range []int{1, 2, 4, 8, 16, 32} {
		b.Run(fmt.Sprintf("bw=%d", bw), func(b *testing.B) {
			A, rhs := makeBandSPD(n, bw)
			nnz := A.nnz()
			b.ReportMetric(float64(n), "n")
			b.ReportMetric(float64(nnz), "nnz")
			b.ResetTimer()
			var totalNS int64
			for i := 0; i < b.N; i++ {
				t0 := time.Now()
				if _, err := solve(A, rhs); err != nil {
					b.Fatal(err)
				}
				totalNS += time.Since(t0).Nanoseconds()
			}
			b.ReportMetric(float64(totalNS)/float64(b.N)/float64(nnz), "ns/nnz")
		})
	}
}
