#ifndef GOWASM_ENGINE_SUPERLU_WRAPPER_H
#define GOWASM_ENGINE_SUPERLU_WRAPPER_H

// solve_superlu_csc solves A*x = b for a sparse, real, double-precision A
// given in 0-indexed CSC (compressed sparse column) format, using SuperLU's
// dgssv driver (COLAMD ordering + partial-pivoted sparse LU).
//
// Inputs:
//   n, nnz          - matrix dimension and number of structural non-zeros
//   values, rowIdx  - CSC value/row-index arrays, length nnz
//   colPtr          - CSC column-pointer array, length n+1
//   rhs             - right-hand side vector, length n
// Output:
//   solution        - overwritten with x, length n (may alias rhs's backing
//                      buffer; caller must have copied rhs in before calling)
// Returns:
//   0 on success. A value in [1, n] means U is exactly singular at that
//   pivot (solution is still written but is not meaningful). A value > n
//   means memory allocation failed.
int solve_superlu_csc(int n, int nnz,
                       double *values, int *rowIdx, int *colPtr,
                       double *rhs, double *solution);

#endif
