//go:build cgo

// This file requires CGo and libsuperlu (see superlu_wrapper.c/.h in this
// directory). It is automatically excluded from builds where CGo is
// disabled — notably GOOS=js GOARCH=wasm, where CGO_ENABLED is forced to 0.
// See superlu_cgo_stub.go for the fallback used in that case.
package nr

/*
#cgo CFLAGS: -I/usr/include/superlu
#cgo LDFLAGS: -lsuperlu -lm

#include <stdlib.h>
#include "superlu_wrapper.h"
*/
import "C"

import (
	"fmt"
	"sort"
	"unsafe"

	"gonum.org/v1/gonum/mat"
)

// SuperLUSolver returns a LinearSolver that uses SuperLU (sparse direct LU
// with COLAMD column reordering) to solve A·x = b. A is never expanded into
// a dense matrix, so cost scales with the number of non-zeros rather than n².
// Requires libsuperlu at build and run time; see shell.nix (Nix) or
// plans/superlu-cgo.md (other platforms) for setup.
func SuperLUSolver() LinearSolver {
	return func(A mat.Matrix, b *mat.VecDense) (*mat.VecDense, error) {
		n, _ := A.Dims()
		if n == 0 {
			return mat.NewVecDense(0, nil), nil
		}

		// rowIdx/colPtr must be int32 (not Go's 64-bit int): SuperLU's
		// int_t is a 32-bit C int on all platforms this package targets,
		// and we hand these slices to C via unsafe.Pointer, so the Go and
		// C element sizes must match exactly or the data is silently
		// reinterpreted as garbage.
		values, rowIdx, colPtr := toCSC(A, n)
		if len(values) == 0 {
			return nil, fmt.Errorf("superlu: matrix has no non-zero entries")
		}

		rhs := make([]float64, n)
		for i := 0; i < n; i++ {
			rhs[i] = b.AtVec(i)
		}
		solution := make([]float64, n)

		info := C.solve_superlu_csc(
			C.int(n),
			C.int(len(values)),
			(*C.double)(unsafe.Pointer(&values[0])),
			(*C.int)(unsafe.Pointer(&rowIdx[0])),
			(*C.int)(unsafe.Pointer(&colPtr[0])),
			(*C.double)(unsafe.Pointer(&rhs[0])),
			(*C.double)(unsafe.Pointer(&solution[0])),
		)
		if info != 0 {
			return nil, fmt.Errorf("superlu: dgssv failed (info=%d; see slu_ddefs.h for meaning)", int(info))
		}

		x := mat.NewVecDense(n, solution)
		return x, nil
	}
}

// toCSC converts A into 0-indexed CSC (compressed sparse column) format, as
// required by SuperLU's dCreate_CompCol_Matrix. If A implements NonZeroer
// (true for every SparseMatrix in the network package), this is
// O(nnz log nnz) — dominated by the sort, not by scanning A. Otherwise it
// falls back to an O(n²) scan of A.At(i, j).
func toCSC(A mat.Matrix, n int) (values []float64, rowIdx, colPtr []int32) {
	type entry struct {
		r, c int
		v    float64
	}
	var entries []entry

	if nz, ok := A.(NonZeroer); ok {
		nz.ForEachNonZero(func(i, j int, v float64) {
			if v != 0 {
				entries = append(entries, entry{i, j, v})
			}
		})
	} else {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if v := A.At(i, j); v != 0 {
					entries = append(entries, entry{i, j, v})
				}
			}
		}
	}

	// SuperLU (like all CSC formats) needs entries sorted by column, then
	// by row within each column.
	sort.Slice(entries, func(a, b int) bool {
		if entries[a].c != entries[b].c {
			return entries[a].c < entries[b].c
		}
		return entries[a].r < entries[b].r
	})

	nnz := len(entries)
	values = make([]float64, nnz)
	rowIdx = make([]int32, nnz)
	colPtr = make([]int32, n+1)

	for k, e := range entries {
		values[k] = e.v
		rowIdx[k] = int32(e.r)
		colPtr[e.c+1]++
	}
	for j := 1; j <= n; j++ {
		colPtr[j] += colPtr[j-1]
	}

	return values, rowIdx, colPtr
}
