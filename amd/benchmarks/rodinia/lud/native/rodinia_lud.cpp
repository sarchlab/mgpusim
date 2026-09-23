/*
 * HIP kernels for the Rodinia LUD benchmark (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/rodinia_lud.
 *
 * Blocked LU decomposition of a dense NxN matrix (no pivoting), BSIZE=16.
 * Three kernels:
 *   lud_diagonal   — in-place LU factor of the diagonal 16x16 block
 *   lud_perimeter  — forward/back-solve for row/column perimeter blocks
 *   lud_internal   — Schur-complement update for interior blocks
 *
 * The thread block is always (BSIZE, BSIZE) = (16, 16). We use the constant
 * BSIZE (not blockDim) for indexing. lud_perimeter still reads gridDim.x and
 * lud_internal reads blockIdx.y, so the HIP runtime emits hidden ABI args.
 */
#include "hip/hip_runtime.h"

#define BSIZE 16

extern "C" __global__ void lud_diagonal(float *a, int n, int offset)
{
    __shared__ float s[BSIZE][BSIZE];
    int tx = threadIdx.x, ty = threadIdx.y;

    s[ty][tx] = a[(offset * BSIZE + ty) * n + (offset * BSIZE + tx)];
    __syncthreads();

    for (int k = 0; k < BSIZE - 1; k++) {
        if (ty > k && tx == k)
            s[ty][k] /= s[k][k];
        __syncthreads();
        if (ty > k && tx > k)
            s[ty][tx] -= s[ty][k] * s[k][tx];
        __syncthreads();
    }

    a[(offset * BSIZE + ty) * n + (offset * BSIZE + tx)] = s[ty][tx];
}

extern "C" __global__ void lud_perimeter(float *a, int n, int offset)
{
    __shared__ float dia [BSIZE][BSIZE];
    __shared__ float peri[BSIZE][BSIZE];

    int tx = threadIdx.x, ty = threadIdx.y;
    int half   = gridDim.x / 2;
    bool is_row = (blockIdx.x < (unsigned)half);
    int  idx    = is_row ? (int)blockIdx.x : ((int)blockIdx.x - half);
    int  blk    = offset + idx + 1;

    dia[ty][tx] = a[(offset * BSIZE + ty) * n + (offset * BSIZE + tx)];

    if (is_row)
        peri[ty][tx] = a[(offset * BSIZE + ty) * n + (blk * BSIZE + tx)];
    else
        peri[ty][tx] = a[(blk   * BSIZE + ty) * n + (offset * BSIZE + tx)];
    __syncthreads();

    if (is_row) {
        for (int k = 0; k < BSIZE - 1; k++) {
            if (ty > k)
                peri[ty][tx] -= dia[ty][k] * peri[k][tx];
            __syncthreads();
        }
        a[(offset * BSIZE + ty) * n + (blk * BSIZE + tx)] = peri[ty][tx];
    } else {
        for (int k = 0; k < BSIZE; k++) {
            if (tx == k)
                peri[ty][k] /= dia[k][k];
            __syncthreads();
            if (tx > k)
                peri[ty][tx] -= peri[ty][k] * dia[k][tx];
            __syncthreads();
        }
        a[(blk * BSIZE + ty) * n + (offset * BSIZE + tx)] = peri[ty][tx];
    }
}

extern "C" __global__ void lud_internal(float *a, int n, int offset)
{
    __shared__ float peri_row[BSIZE][BSIZE];
    __shared__ float peri_col[BSIZE][BSIZE];

    int tx = threadIdx.x, ty = threadIdx.y;
    int col_blk = offset + blockIdx.x + 1;
    int row_blk = offset + blockIdx.y + 1;

    peri_row[ty][tx] = a[(offset  * BSIZE + ty) * n + (col_blk * BSIZE + tx)];
    peri_col[ty][tx] = a[(row_blk * BSIZE + ty) * n + (offset  * BSIZE + tx)];
    __syncthreads();

    float sum = 0.0f;
    for (int k = 0; k < BSIZE; k++)
        sum += peri_col[ty][k] * peri_row[k][tx];
    a[(row_blk * BSIZE + ty) * n + (col_blk * BSIZE + tx)] -= sum;
}
