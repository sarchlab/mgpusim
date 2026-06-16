/*
 * HIP kernels for the shared-memory (LDS) bandwidth microbenchmark
 * (gfx942 / CDNA3). Extracted from sarchlab/gpu_benchmarks
 * tier1/shared_mem_bandwidth.
 *
 * Two access patterns over a block-local __shared__ buffer:
 *   - smem_bw_no_conflict : stride-1 per lane (each thread owns a
 *                           disjoint set of cells; bank-conflict free)
 *   - smem_bw_conflict    : stride-32 between lanes (lanes of a warp
 *                           hit the same bank)
 *
 * Block geometry is a compile-time constant (BLOCK_SIZE) and the
 * shared buffer length is a constant (SMEM_FLOATS) so the compiler
 * emits NO hidden ABI arguments (kernarg_segment_size = 16: one
 * 8-byte pointer + one 4-byte int + 4 bytes alignment tail).
 *
 * Each thread accumulates into a private "acc"; thread 0 of each block
 * writes acc to d_sink[blockIdx.x]. Because the no_conflict pattern
 * gives every thread a disjoint set of cells, the result is fully
 * deterministic and reproducible on the CPU.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE  64
#define SMEM_FLOATS 512

extern "C" __global__ void smem_bw_no_conflict(float *d_sink, int inner_iters)
{
    __shared__ float smem[SMEM_FLOATS];

    const int tid = threadIdx.x;
    const int n   = BLOCK_SIZE;

    for (int i = tid; i < SMEM_FLOATS; i += n)
        smem[i] = 1.0f;
    __syncthreads();

    float acc = 0.0f;
    for (int it = 0; it < inner_iters; ++it) {
        for (int i = tid; i < SMEM_FLOATS; i += n) {
            acc += smem[i];   // read
            smem[i] = acc;    // write
        }
    }
    __syncthreads();

    if (tid == 0)
        d_sink[blockIdx.x] = acc;
}

extern "C" __global__ void smem_bw_conflict(float *d_sink, int inner_iters)
{
    __shared__ float smem[SMEM_FLOATS];

    const int tid  = threadIdx.x;
    const int n    = BLOCK_SIZE;
    const int lane = tid & 31;
    const int warp = tid >> 5;
    const int nw   = n >> 5;                  // number of warps

    for (int i = tid; i < SMEM_FLOATS; i += n)
        smem[i] = 1.0f;
    __syncthreads();

    const int elems_per_thread = SMEM_FLOATS / n;
    float acc = 0.0f;

    for (int it = 0; it < inner_iters; ++it) {
        for (int j = 0; j < elems_per_thread; ++j) {
            int idx = (lane * 32 + warp + j * nw) % SMEM_FLOATS;
            acc += smem[idx];   // read  — all lanes hit same bank
            smem[idx] = acc;    // write — all lanes hit same bank
        }
    }
    __syncthreads();

    if (tid == 0)
        d_sink[blockIdx.x] = acc;
}
