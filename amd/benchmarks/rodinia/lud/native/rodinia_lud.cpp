// GPU Benchmarks v1.1, tier2/rodinia_lud/hip/rodinia_lud.hip.
#include <hip/hip_runtime.h>

#define BSIZE 16

extern "C" __global__ void lud_diagonal(float *a, int n, int offset)
{
    __shared__ float s[BSIZE][BSIZE];
    int tx = threadIdx.x, ty = threadIdx.y;

    s[ty][tx] = a[(offset * BSIZE + ty) * n + (offset * BSIZE + tx)];
    __syncthreads();

    for (int k = 0; k < BSIZE - 1; k++) {
        // Normalise column k below pivot  → L[i][k] = a[i][k] / a[k][k]
        if (ty > k && tx == k)
            s[ty][k] /= s[k][k];
        __syncthreads();
        // Schur complement on trailing sub-block
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
    bool is_row = (blockIdx.x < half);
    int  idx    = is_row ? blockIdx.x : (blockIdx.x - half);
    int  blk    = offset + idx + 1;   // target block index

    // All threads in this block load the same diagonal block
    dia[ty][tx] = a[(offset * BSIZE + ty) * n + (offset * BSIZE + tx)];

    if (is_row)
        peri[ty][tx] = a[(offset * BSIZE + ty) * n + (blk * BSIZE + tx)];
    else
        peri[ty][tx] = a[(blk   * BSIZE + ty) * n + (offset * BSIZE + tx)];
    __syncthreads();

    // is_row is uniform across the entire thread block (depends only on blockIdx.x),
    // so __syncthreads() inside these branches is valid.
    if (is_row) {
        // Forward substitution: unit-lower-triangular L * peri = peri
        for (int k = 0; k < BSIZE - 1; k++) {
            if (ty > k)
                peri[ty][tx] -= dia[ty][k] * peri[k][tx];
            __syncthreads();
        }
        a[(offset * BSIZE + ty) * n + (blk * BSIZE + tx)] = peri[ty][tx];
    } else {
        // Back substitution: peri * U = peri  → find L column
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
    __shared__ float peri_row[BSIZE][BSIZE];  // U[offset, col_blk]
    __shared__ float peri_col[BSIZE][BSIZE];  // L[row_blk, offset]

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
