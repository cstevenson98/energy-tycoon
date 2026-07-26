package network

import (
	"sort"

	"gonum.org/v1/gonum/mat"
)

// SparseMatrix is a real sparse matrix whose non-zero structure is fixed at
// construction time. It combines a CSR (Compressed Sparse Row) layout for
// fast row iteration with a hash map for O(1) random access and in-place
// value updates.
//
// Zero() + per-entry Set() on the same pre-allocated object is the intended
// update pattern inside a Newton-Raphson loop: the structure never changes,
// only the values.
type SparseMatrix struct {
	r, c    int
	data    []float64 // non-zero values (CSR order: row-major)
	colIdx  []int     // column index of each entry in data
	rowPtr  []int     // rowPtr[i] = first index in data for row i; rowPtr[r] = nnz
	pos     map[uint64]int // pack(row,col) → index in data (for Set / At)
}

// Compile-time check: SparseMatrix implements mat.Matrix.
var _ mat.Matrix = (*SparseMatrix)(nil)

// Dims returns the matrix dimensions.
func (m *SparseMatrix) Dims() (r, c int) { return m.r, m.c }

// At returns the value at (i, j), or 0 if (i, j) is a structural zero.
func (m *SparseMatrix) At(i, j int) float64 {
	if idx, ok := m.pos[packRC(i, j)]; ok {
		return m.data[idx]
	}
	return 0
}

// T returns a transposed view (satisfies mat.Matrix; does not reallocate).
func (m *SparseMatrix) T() mat.Matrix { return mat.Transpose{Matrix: m} }

// Set overwrites the stored value at (i, j). Panics if (i, j) is not a
// structural non-zero (callers must only write to pattern positions).
func (m *SparseMatrix) Set(i, j int, v float64) {
	m.data[m.pos[packRC(i, j)]] = v
}

// Add adds v to the stored value at (i, j). Panics if (i, j) is not
// a structural non-zero.
func (m *SparseMatrix) Add(i, j int, v float64) {
	m.data[m.pos[packRC(i, j)]] += v
}

// Zero resets all stored values to 0 in O(nnz).
func (m *SparseMatrix) Zero() {
	for k := range m.data {
		m.data[k] = 0
	}
}

// NNZ returns the number of structural non-zeros.
func (m *SparseMatrix) NNZ() int { return len(m.data) }

// ForEachInRow calls f(j, v) for every structural non-zero in row i, in
// ascending column order. v may be 0 if Set has not been called.
func (m *SparseMatrix) ForEachInRow(i int, f func(j int, v float64)) {
	for k := m.rowPtr[i]; k < m.rowPtr[i+1]; k++ {
		f(m.colIdx[k], m.data[k])
	}
}

// ForEachNonZero calls f(i, j, v) for every structural entry in row-major
// order. Used by SuperLUSolver's toCSC path (O(nnz)).
func (m *SparseMatrix) ForEachNonZero(f func(i, j int, v float64)) {
	for i := 0; i < m.r; i++ {
		for k := m.rowPtr[i]; k < m.rowPtr[i+1]; k++ {
			f(i, m.colIdx[k], m.data[k])
		}
	}
}

// packRC packs a (row, col) pair into a single uint64 key.
// Rows and columns must fit in 32 bits (safe for any realistic power network).
func packRC(r, c int) uint64 { return uint64(uint32(r))<<32 | uint64(uint32(c)) }

// NewSparseFromPattern creates a zero-valued SparseMatrix from a slice of
// (row, col) pairs. Duplicate pairs are silently deduplicated. The resulting
// matrix has the exact non-zero structure of the input pattern.
func NewSparseFromPattern(nRows, nCols int, pairs [][2]int) *SparseMatrix {
	// Deduplicate while preserving all unique (row, col) pairs.
	seen := make(map[uint64]struct{}, len(pairs))
	uniq := make([][2]int, 0, len(pairs))
	for _, p := range pairs {
		k := packRC(p[0], p[1])
		if _, dup := seen[k]; !dup {
			seen[k] = struct{}{}
			uniq = append(uniq, p)
		}
	}

	// Sort by row then column so CSR rowPtr can be built with a prefix sum.
	sort.Slice(uniq, func(a, b int) bool {
		if uniq[a][0] != uniq[b][0] {
			return uniq[a][0] < uniq[b][0]
		}
		return uniq[a][1] < uniq[b][1]
	})

	nnz := len(uniq)
	m := &SparseMatrix{
		r:      nRows,
		c:      nCols,
		data:   make([]float64, nnz),
		colIdx: make([]int, nnz),
		rowPtr: make([]int, nRows+1),
		pos:    make(map[uint64]int, nnz),
	}

	// Build colIdx and pos map.
	for k, p := range uniq {
		m.colIdx[k] = p[1]
		m.pos[packRC(p[0], p[1])] = k
	}

	// Build rowPtr via a count-then-prefix-sum pass.
	for _, p := range uniq {
		m.rowPtr[p[0]+1]++
	}
	for i := 1; i <= nRows; i++ {
		m.rowPtr[i] += m.rowPtr[i-1]
	}

	return m
}
