/*
 * HIP kernels for the shared-memory (LDS) BANDWIDTH microbenchmark
 * (gfx942 / CDNA3). Authored for the MGPUSim MI300X calibration.
 *
 * Unlike the latency variant (sharedmemlatency: single accumulator -> serial
 * dependent chain), this kernel issues UNROLL independent LDS reads per
 * iteration into UNROLL independent register accumulators, so many LDS
 * accesses are in flight at once and the LDS unit can be saturated -- i.e. it
 * measures bandwidth, not latency. Launch with a full block (256 threads = 4
 * wavefronts) and many blocks (>= number of CUs) so every CU's LDS is busy.
 *
 * Forcing real ds_read traffic took two things (constant LDS data gets folded
 * away by the compiler, even through volatile):
 *   1. Stage the buffer from a GLOBAL input (d_in) -> contents are opaque to
 *      the compiler, so the reads can't be constant-folded.
 *   2. Make the read index depend on the loop counter (it) -> the reads are
 *      not loop-invariant, so they can't be hoisted out of the loop.
 * d_in is filled with 1.0 on the host, so each block's result is exactly
 * UNROLL * inner_iters -- deterministic and CPU-reproducible -- but the
 * compiler can't know that and must emit inner_iters * UNROLL ds_reads.
 *
 *   - smem_bw_no_conflict : consecutive lanes -> consecutive banks (conflict-free)
 *   - smem_bw_conflict    : 32 lanes -> the same bank (32-way bank conflict)
 *
 * Signature: (const float *d_in, float *d_sink, int inner_iters).
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE  256
#define UNROLL      8
#define SMEM_FLOATS (BLOCK_SIZE * UNROLL)   // 2048, power of two (mask-wrappable)

extern "C" __global__ void smem_bw_no_conflict(const float *d_in,
                                               float *d_sink, int inner_iters)
{
    __shared__ float smem[SMEM_FLOATS];
    const int tid = threadIdx.x;

    // Contiguous per-thread staging from the opaque global input. Contiguous
    // writes compile to ds_write_b128; a strided init would emit
    // ds_write2st64_b32, which the CDNA3 emulator mishandles. The streaming
    // reads below are still strided (conflict-free) -- only the one-time init
    // pattern changes.
    const int base = tid * UNROLL;
#pragma unroll
    for (int u = 0; u < UNROLL; ++u)
        smem[base + u] = d_in[base + u];
    __syncthreads();

    float acc[UNROLL];
#pragma unroll
    for (int u = 0; u < UNROLL; ++u)
        acc[u] = 0.0f;

    for (int it = 0; it < inner_iters; ++it) {
#pragma unroll
        for (int u = 0; u < UNROLL; ++u) {
            // it-varying window (not hoistable); consecutive lanes ->
            // consecutive cells -> consecutive banks (conflict-free).
            int idx = (tid + (u + it) * BLOCK_SIZE) & (SMEM_FLOATS - 1);
            acc[u] += smem[idx];
        }
    }
    __syncthreads();

    float sum = 0.0f;
#pragma unroll
    for (int u = 0; u < UNROLL; ++u)
        sum += acc[u];

    if (tid == 0)
        d_sink[blockIdx.x] = sum;   // == UNROLL * inner_iters (d_in is all 1.0)
}

extern "C" __global__ void smem_bw_conflict(const float *d_in,
                                            float *d_sink, int inner_iters)
{
    __shared__ float smem[SMEM_FLOATS];
    const int tid  = threadIdx.x;
    const int lane = tid & 31;

    // Contiguous per-thread staging (see no_conflict): avoids ds_write2st64_b32.
    const int base = tid * UNROLL;
#pragma unroll
    for (int u = 0; u < UNROLL; ++u)
        smem[base + u] = d_in[base + u];
    __syncthreads();

    float acc[UNROLL];
#pragma unroll
    for (int u = 0; u < UNROLL; ++u)
        acc[u] = 0.0f;

    for (int it = 0; it < inner_iters; ++it) {
#pragma unroll
        for (int u = 0; u < UNROLL; ++u) {
            // stride 32 floats between lanes -> all 32 lanes of a warp hit the
            // same bank (32-way conflict); (u+it) shifts which bank, and makes
            // the access it-varying so it can't be hoisted.
            int idx = (lane * 32 + ((u + it) & 31)) & (SMEM_FLOATS - 1);
            acc[u] += smem[idx];
        }
    }
    __syncthreads();

    float sum = 0.0f;
#pragma unroll
    for (int u = 0; u < UNROLL; ++u)
        sum += acc[u];

    if (tid == 0)
        d_sink[blockIdx.x] = sum;
}
