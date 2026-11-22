/*
 * HIP kernels for PolyBench Gram-Schmidt (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_gramschmidt.
 *
 * Computes the QR factorization via Gram-Schmidt orthogonalization:
 *   A = Q * R, A is M x N, Q is M x N (orthonormal columns), R is N x N.
 *
 * The host iterates k = 0 .. N-1 and launches these kernels per column:
 *   gram_norm_finish : single thread computes nrm = sqrt(sum A[:,k]^2),
 *                      stores R[k,k] = nrm and nrm_buf[0] = nrm
 *   gram_normalize   : M threads, Q[:,k] = A[:,k] / nrm_buf[0]
 *   gram_project     : one thread per column j > k:
 *                        dot = Q[:,k] . A[:,j]; R[k,j] = dot;
 *                        A[:,j] -= dot * Q[:,k]
 *
 * The original PolyBench kernel computed the column norm with atomicAdd.
 * The CDNA3 functional emulator does not implement global atomics, so the
 * norm is computed sequentially in the single-thread gram_norm_finish
 * kernel instead (numerically identical to a serial reduction).
 *
 * Block geometry uses a constant BLOCK_SIZE (not blockDim) so the compiler
 * emits no hidden ABI arguments.
 */
#include "hip/hip_runtime.h"
#include <math.h>

#define BLOCK_SIZE 256

// Only the first work-item computes nrm = sqrt(sum_i A[i*N+k]^2),
// storing into R[k*N+k] and nrm_buf[0]. Launched with a full block; all
// other lanes early-exit.
extern "C" __global__ void gram_norm_finish(const float* __restrict__ A,
                                            float* __restrict__ R,
                                            float* __restrict__ nrm_buf,
                                            int M, int N, int k)
{
    int gid = BLOCK_SIZE * blockIdx.x + threadIdx.x;
    if (gid != 0) return;

    float sum = 0.0f;
    for (int i = 0; i < M; i++) {
        float v = A[i * N + k];
        sum += v * v;
    }
    float nrm = sqrtf(sum);
    R[k * N + k] = nrm;
    nrm_buf[0] = nrm;
}

// Q[:,k] = A[:,k] / nrm_buf[0]
extern "C" __global__ void gram_normalize(const float* __restrict__ A,
                                          float* __restrict__ Q,
                                          const float* __restrict__ nrm_buf,
                                          int M, int N, int k)
{
    int i = BLOCK_SIZE * blockIdx.x + threadIdx.x;
    if (i >= M) return;
    Q[i * N + k] = A[i * N + k] / nrm_buf[0];
}

// For each column j > k:
//   dot = Q[:,k] . A[:,j]; R[k,j] = dot; A[:,j] -= dot * Q[:,k]
extern "C" __global__ void gram_project(float* __restrict__ A,
                                        const float* __restrict__ Q,
                                        float* __restrict__ R,
                                        int M, int N, int k)
{
    int j = BLOCK_SIZE * blockIdx.x + threadIdx.x + k + 1;
    if (j >= N) return;

    float dot = 0.0f;
    for (int i = 0; i < M; i++)
        dot += Q[i * N + k] * A[i * N + j];
    R[k * N + j] = dot;
    for (int i = 0; i < M; i++)
        A[i * N + j] -= dot * Q[i * N + k];
}
