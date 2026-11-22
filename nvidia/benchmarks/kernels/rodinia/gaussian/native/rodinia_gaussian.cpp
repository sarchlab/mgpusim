/*
 * HIP kernels for the Rodinia Gaussian elimination benchmark (gfx942 / CDNA3).
 * Extracted from sarchlab/gpu_benchmarks tier2/rodinia_gaussian.
 *
 * Two kernels perform GPU forward elimination of a dense NxN system Ax=b:
 *   fan1: compute multipliers m[i][k] = a[i][k] / a[k][k] for pivot column t
 *   fan2: update submatrix below pivot row t, and the rhs vector b
 *
 * Block dimensions are CONSTANT literals (BLOCK1D for fan1, BLOCK2D x BLOCK2D
 * for fan2) instead of blockDim.x/y, so the compiler emits no hidden ABI
 * arguments. Each kernel reads only blockIdx/threadIdx.
 */
#include "hip/hip_runtime.h"

#define BLOCK1D 256
#define BLOCK2D 16

// fan1 — compute multipliers for pivot column t.
// Each thread handles one row below the pivot.
extern "C" __global__ void fan1(float* __restrict__ m,
                                const float* __restrict__ a,
                                int Size, int t)
{
    int tid = blockIdx.x * BLOCK1D + threadIdx.x;
    if (tid < Size - 1 - t) {
        int row = t + 1 + tid;
        m[row * Size + t] = a[row * Size + t] / a[t * Size + t];
    }
}

// fan2 — eliminate pivot column from submatrix below pivot row t.
// Each (col, row) thread updates one submatrix element; col==0 also updates b.
extern "C" __global__ void fan2(const float* __restrict__ m,
                                float* __restrict__ a,
                                float* __restrict__ b,
                                int Size, int t)
{
    int col = blockIdx.x * BLOCK2D + threadIdx.x;  // relative column index
    int row = blockIdx.y * BLOCK2D + threadIdx.y;  // relative row index
    int remaining = Size - t - 1;

    if (col < remaining && row < remaining) {
        int abs_row = t + 1 + row;
        int abs_col = t + 1 + col;
        a[abs_row * Size + abs_col] -= m[abs_row * Size + t] * a[t * Size + abs_col];
        if (col == 0) {
            b[abs_row] -= m[abs_row * Size + t] * b[t];
        }
    }
}
