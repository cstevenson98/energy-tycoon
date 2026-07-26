# SuperLU CGo Integration Plan

## Overview

Replace the O(n³) dense LU factorization in `pkg/nr/nr.go` with a true sparse direct solver via CGo bindings to SuperLU. SuperLU is a mature, production-grade library for solving sparse linear systems Ax = b using LU decomposition with partial pivoting and column reordering for sparsity preservation.

**Key benefits:**
- Complexity drops from O(n³) dense to ~O(n^1.5) sparse for typical power network sparsity
- Robust convergence (direct method, no tuning needed unlike iterative solvers)
- Preserves sparsity throughout factorization (column AMD/COLAMD ordering)
- Battle-tested in power systems, circuit simulation, FEM

---

## Architecture

### 1. SuperLU Library

**What is SuperLU:**
- C library: `libsuperlu.so` / `libsuperlu.a` (LGPL or BSD depending on version)
- Solves `A·x = b` where A is sparse, real or complex
- CSC (Compressed Sparse Column) input format
- Handles rectangular, singular, or rank-deficient systems gracefully

**How to obtain:**
- **NixOS/Nix**: Already configured in root `shell.nix` (pkgs.superlu)
- **Ubuntu/Debian**: `sudo apt-get install libsuperlu-dev`
- **macOS (Homebrew)**: `brew install superlu`
- **From source**: https://github.com/xiaoyeli/superlu (tag v5.3.0 or later)

**Headers needed:**
- `slu_ddefs.h` — double-precision real API
- `supermatrix.h` — sparse matrix storage types

---

## 2. Go Package Structure

```
pkg/nr/
├── nr.go                    # existing NR solver (unchanged interface)
├── nr_test.go               # existing tests (all should still pass)
├── sparse_lu.go             # dense fallback (existing SparseLUSolver, renamed)
├── superlu_cgo.go           # new: CGo wrapper (+build !nowasm)
├── superlu_cgo_stub.go      # new: no-op stub (+build nowasm)
└── superlu/
    ├── superlu.h            # C shim header (optional, can use system headers)
    └── superlu_wrapper.c    # C helper for dgstrf/dgstrs calls
```

**Why the stub?** WASM builds (`GOOS=js GOARCH=wasm`) cannot use CGo. The stub provides a compile-time fallback that panics or returns an error at runtime.

---

## 3. CGo Wrapper API

### 3.1. Go interface (in `superlu_cgo.go`)

```go
// +build !nowasm

package nr

/*
#cgo CFLAGS: -I/usr/include/superlu
#cgo LDFLAGS: -lsuperlu -lm

#include <stdlib.h>
#include "slu_ddefs.h"

// C wrapper function (defined in superlu_wrapper.c)
int solve_superlu_csc(int n, int nnz,
                      double *values, int *rowIdx, int *colPtr,
                      double *rhs, double *solution);
*/
import "C"
import (
	"fmt"
	"unsafe"

	"gonum.org/v1/gonum/mat"
)

// SuperLUSolver returns a LinearSolver that uses SuperLU for sparse direct solve.
// Requires libsuperlu.so at runtime. Panics if called in a WASM build.
func SuperLUSolver() LinearSolver {
	return func(A mat.Matrix, b *mat.VecDense) (*mat.VecDense, error) {
		n, _ := A.Dims()

		// Convert A to CSC format (required by SuperLU).
		values, rowIdx, colPtr := toCSC(A, n)

		// Allocate solution vector.
		x := mat.NewVecDense(n, nil)
		solution := make([]float64, n)
		for i := 0; i < n; i++ {
			solution[i] = b.AtVec(i)
		}

		// Call C wrapper (modifies solution in-place).
		ret := C.solve_superlu_csc(
			C.int(n),
			C.int(len(values)),
			(*C.double)(unsafe.Pointer(&values[0])),
			(*C.int)(unsafe.Pointer(&rowIdx[0])),
			(*C.int)(unsafe.Pointer(&colPtr[0])),
			(*C.double)(unsafe.Pointer(&solution[0])),
			(*C.double)(unsafe.Pointer(&solution[0])),
		)

		if ret != 0 {
			return nil, fmt.Errorf("superlu: factorization failed (code %d)", ret)
		}

		for i := 0; i < n; i++ {
			x.SetVec(i, solution[i])
		}
		return x, nil
	}
}

// toCSC converts a mat.Matrix (typically our *SparseMatrix) to CSC format.
// If A implements NonZeroer, this is O(nnz); otherwise O(n²).
func toCSC(A mat.Matrix, n int) (values []float64, rowIdx, colPtr []int) {
	// Step 1: collect all non-zeros as (row, col, val) triples.
	type entry struct{ r, c int; v float64 }
	entries := []entry{}

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

	// Step 2: sort by (col, row) to build CSC.
	sort.Slice(entries, func(a, b int) bool {
		if entries[a].c != entries[b].c {
			return entries[a].c < entries[b].c
		}
		return entries[a].r < entries[b].r
	})

	// Step 3: extract values, rowIdx, colPtr.
	nnz := len(entries)
	values = make([]float64, nnz)
	rowIdx = make([]int, nnz)
	colPtr = make([]int, n+1)

	for k, e := range entries {
		values[k] = e.v
		rowIdx[k] = e.r
		colPtr[e.c+1]++
	}
	for j := 1; j <= n; j++ {
		colPtr[j] += colPtr[j-1]
	}

	return
}
```

### 3.2. C wrapper (in `superlu_wrapper.c`)

```c
#include <stdlib.h>
#include "slu_ddefs.h"

// solve_superlu_csc performs A·x = b using SuperLU.
// Input:
//   n, nnz: matrix dimensions
//   values, rowIdx, colPtr: CSC format (0-indexed)
//   rhs: right-hand side (length n)
// Output:
//   solution: overwritten with x (length n)
// Returns: 0 on success, non-zero on failure.
int solve_superlu_csc(int n, int nnz,
                      double *values, int *rowIdx, int *colPtr,
                      double *rhs, double *solution) {
    SuperMatrix A, L, U, B;
    int *perm_r, *perm_c;
    superlu_options_t options;
    SuperLUStat_t stat;
    int info;

    // Create sparse matrix A in CSC format.
    dCreate_CompCol_Matrix(&A, n, n, nnz, values, rowIdx, colPtr, 
                           SLU_NC, SLU_D, SLU_GE);

    // Create dense RHS matrix B (nrhs=1).
    dCreate_Dense_Matrix(&B, n, 1, solution, n, SLU_DN, SLU_D, SLU_GE);
    for (int i = 0; i < n; i++) {
        solution[i] = rhs[i];
    }

    // Allocate permutation arrays.
    perm_r = malloc(n * sizeof(int));
    perm_c = malloc(n * sizeof(int));
    if (!perm_r || !perm_c) {
        return -1;
    }

    // Set default options (COLAMD ordering, partial pivoting).
    set_default_options(&options);
    options.ColPerm = COLAMD;

    // Initialize statistics.
    StatInit(&stat);

    // Solve A·x = B.
    dgssv(&options, &A, perm_c, perm_r, &L, &U, &B, &stat, &info);

    // Cleanup.
    Destroy_SuperMatrix_Store(&A);
    Destroy_SuperMatrix_Store(&B);
    Destroy_SuperNode_Matrix(&L);
    Destroy_CompCol_Matrix(&U);
    StatFree(&stat);
    free(perm_r);
    free(perm_c);

    return info;
}
```

### 3.3. Stub for WASM (in `superlu_cgo_stub.go`)

```go
// +build nowasm

package nr

import (
	"fmt"

	"gonum.org/v1/gonum/mat"
)

// SuperLUSolver panics in WASM builds because CGo is unavailable.
// Use SparseLUSolver (dense fallback) instead.
func SuperLUSolver() LinearSolver {
	return func(A mat.Matrix, b *mat.VecDense) (*mat.VecDense, error) {
		return nil, fmt.Errorf("superlu: CGo not available in WASM build")
	}
}
```

---

## 4. Integration into LoadflowSolver

Update `game/components/network/solver.go`:

```go
nrSolver := &nr.NewtonRaphson{
	MaxIter: maxIter,
	Tol:     tol,
	LinearSolve: nr.SuperLUSolver(), // <-- was nr.SparseLUSolver()
}
```

**Fallback logic** (optional, for robustness):

```go
solver := nr.SuperLUSolver()
// If SuperLU is unavailable (stub or missing .so), fall back to dense.
if solver == nil {
	solver = nr.SparseLUSolver()
}
nrSolver.LinearSolve = solver
```

---

## 5. Build System

### 5.1. Native (Linux/macOS)

**NixOS/Nix (already configured):**
```bash
# Enter the nix shell from repo root
nix-shell

# CGO flags are set automatically via shellHook
cd examples/grid-sim-game
go build ./...
```

**Ubuntu/Debian:**
```bash
sudo apt-get install libsuperlu-dev
cd examples/grid-sim-game
go build ./...
```

**macOS:**
```bash
brew install superlu
cd examples/grid-sim-game
go build ./...
```

CGo will automatically link `-lsuperlu` via the `#cgo LDFLAGS` directive (or via shell.nix for Nix users).

### 5.2. WASM

```bash
cd examples/grid-sim-game
GOOS=js GOARCH=wasm go build -tags nowasm -o game/main.wasm game/main.go
```

The `-tags nowasm` activates `superlu_cgo_stub.go` instead of the CGo version. The solver will fall back to dense LU (or return an error if you force SuperLU).

---

## 6. Testing Strategy

### 6.1. Unit tests (extend `pkg/nr/nr_test.go`)

```go
func TestSuperLUSolver(t *testing.T) {
	// Same test cases as TestLinearSystem, TestNonlinearSystem,
	// but with SuperLUSolver() instead of SparseLUSolver().
	// Verify identical numerical results.
}
```

### 6.2. Benchmark

```go
func BenchmarkSuperLU(b *testing.B) {
	// Build a 200×200 sparse Jacobian (typical for 100-bus network).
	// Compare:
	//   - Dense LU (baseline)
	//   - SuperLU
	// Expect ~10–50× speedup for realistic sparsity.
}
```

### 6.3. Integration test (network solver)

All existing `solver_test.go` tests should pass unchanged — they call `LoadflowSolver.Solve`, which now uses SuperLU under the hood.

---

## 7. Deployment Considerations

### 7.1. Library availability

**Docker/CI:**
```dockerfile
RUN apt-get update && apt-get install -y libsuperlu-dev
```

**User machines:**
- Linux: package manager (`apt`, `yum`, `pacman`)
- macOS: Homebrew
- Windows: Build from source or use MinGW; alternatively, vendor a static `.a`

### 7.2. Static linking (optional)

To avoid runtime `.so` dependency:

```go
// #cgo LDFLAGS: /usr/lib/x86_64-linux-gnu/libsuperlu.a -lgfortran -lm
```

This embeds SuperLU directly into the Go binary. Requires the static archive and Fortran runtime (SuperLU's BLAS/LAPACK dependencies).

---

## 8. Alternative: KLU (SuiteSparse)

If SuperLU proves difficult to integrate, consider **KLU** from SuiteSparse:
- Designed specifically for circuit/power network matrices
- Simpler API (fewer dependencies)
- Header: `klu.h`
- LGPL license

The CGo wrapper would be nearly identical, just replace `dgssv` with `klu_analyze` + `klu_factor` + `klu_solve`.

---

## 9. Implementation Checklist

- [ ] Install libsuperlu-dev on dev machine
- [ ] Create `pkg/nr/superlu_wrapper.c` with C shim
- [ ] Create `pkg/nr/superlu_cgo.go` with Go wrapper
- [ ] Create `pkg/nr/superlu_cgo_stub.go` for WASM
- [ ] Rename existing `SparseLUSolver` → `DenseLUSolver` for clarity
- [ ] Update `solver.go` to call `nr.SuperLUSolver()`
- [ ] Add `TestSuperLUSolver` unit test
- [ ] Add `BenchmarkSuperLU` vs dense
- [ ] Run full test suite (`go test ./...`)
- [ ] Update `go.mod` if needed (no Go deps added, just C lib)
- [ ] Document in README: "Requires libsuperlu.so for native builds"
- [ ] Test WASM build with `-tags nowasm`
- [ ] (Optional) Add Docker/CI setup with libsuperlu-dev

---

## 10. Expected Performance

**2-bus network (n=1, nnz≈3):**
- No measurable difference (both sub-microsecond)

**100-bus network (n≈100, nnz≈400):**
- Dense LU: ~1 ms per iteration
- SuperLU: ~0.05 ms per iteration (~20× faster)

**1000-bus network (n≈1000, nnz≈4000):**
- Dense LU: ~1000 ms per iteration (unusable for real-time)
- SuperLU: ~5 ms per iteration (~200× faster)

The crossover is around n=50–100; below that, dense is competitive due to better cache locality.

---

## Notes

- SuperLU is thread-safe for independent solves (multiple `dgssv` calls on different matrices), but a single solve is single-threaded. For multi-core, consider PARDISO or PaStiX (more complex to integrate).
- The CSC conversion (`toCSC`) is O(nnz log nnz) due to sorting. Could cache the CSC representation and only update values between NR iterations (same structure optimization we already do for the Jacobian).
- If SuperLU fails (singular matrix, out of memory), the wrapper should return a clear error — the NR solver will propagate it up and the game can display "Load flow diverged".
