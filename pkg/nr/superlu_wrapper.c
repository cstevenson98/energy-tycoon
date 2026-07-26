#include "superlu_wrapper.h"

#include <stdlib.h>

#include "slu_ddefs.h"

int solve_superlu_csc(int n, int nnz,
                       double *values, int *rowIdx, int *colPtr,
                       double *rhs, double *solution) {
    SuperMatrix A, L, U, B;
    int *perm_r = NULL, *perm_c = NULL;
    superlu_options_t options;
    SuperLUStat_t stat;
    int info = 0;

    // Wrap the caller's CSC arrays (no copy). Column-wise, no supernode.
    dCreate_CompCol_Matrix(&A, n, n, nnz, values, rowIdx, colPtr,
                            SLU_NC, SLU_D, SLU_GE);

    // B is the dense RHS/solution matrix (nrhs = 1); dgssv overwrites it
    // with x in place, so copy rhs into solution's backing buffer first.
    for (int i = 0; i < n; i++) {
        solution[i] = rhs[i];
    }
    dCreate_Dense_Matrix(&B, n, 1, solution, n, SLU_DN, SLU_D, SLU_GE);

    perm_r = malloc((size_t)n * sizeof(int));
    perm_c = malloc((size_t)n * sizeof(int));
    if (!perm_r || !perm_c) {
        free(perm_r);
        free(perm_c);
        Destroy_SuperMatrix_Store(&A);
        Destroy_SuperMatrix_Store(&B);
        return -1;
    }

    set_default_options(&options);
    options.ColPerm = COLAMD;

    StatInit(&stat);

    dgssv(&options, &A, perm_c, perm_r, &L, &U, &B, &stat, &info);

    // A and B wrap caller-owned buffers: only free the SuperMatrix "store"
    // wrapper, never the underlying values/rowIdx/colPtr/solution arrays.
    Destroy_SuperMatrix_Store(&A);
    Destroy_SuperMatrix_Store(&B);

    // L and U are allocated by dgssv itself. They exist whenever info <= n
    // (info in [1, n] means U is singular but was still fully computed);
    // info > n indicates a malloc failure inside dgssv, in which case L/U
    // may be only partially built and must not be freed here.
    if (info <= n) {
        Destroy_SuperNode_Matrix(&L);
        Destroy_CompCol_Matrix(&U);
    }

    StatFree(&stat);
    free(perm_r);
    free(perm_c);

    return info;
}
