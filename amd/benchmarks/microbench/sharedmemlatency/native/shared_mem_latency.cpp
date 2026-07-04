/*
 * HIP kernels for the shared-memory (LDS) dependent-latency microbenchmark
 * (gfx942 / CDNA3). Extracted from sarchlab/gpu_benchmarks
 * tier1/shared_mem_bandwidth.
 *
 * Two access patterns over a block-local __shared__ buffer:
 *   - smem_lat_no_conflict : stride-1 per lane (each thread owns a
 *                           disjoint set of cells; bank-conflict free)
 *   - smem_lat_conflict    : stride-32 between lanes (lanes of a warp
 *                           hit the same bank)
 *
 * The block size is passed as an explicit kernel argument (block_size)
 * because the MGPUSim CDNA3 emulator does not populate blockDim.x; the
 * shared-buffer length SMEM_FLOATS (12288 = 48 KB) matches the real
 * gpu_benchmarks kernel so the sim does the same amount of LDS work.
 * kernarg_segment_size = 16: one 8-byte pointer + two 4-byte ints
 * (inner_iters, block_size), no padding.
 *
 * Each thread accumulates into a private "acc"; thread 0 of each block
 * writes acc to d_sink[blockIdx.x]. Because the no_conflict pattern
 * gives every thread a disjoint set of cells, the result is fully
 * deterministic and reproducible on the CPU.
 */
#include "hip/hip_runtime.h"

#define SMEM_FLOATS 12288

extern "C" __global__ void smem_lat_no_conflict(float *d_sink, int inner_iters, int block_size)
{
    __shared__ float smem[SMEM_FLOATS];

    const int tid = threadIdx.x;
    const int n   = block_size;

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

extern "C" __global__ void smem_lat_conflict(float *d_sink, int inner_iters, int block_size)
{
    __shared__ float smem[SMEM_FLOATS];

    const int tid  = threadIdx.x;
    const int n    = block_size;
    const int lane = tid & 31;
    const int warp = tid >> 5;
    const int nw   = n >> 5;                  // number of warps

    for (int i = tid; i < SMEM_FLOATS; i += n)
        smem[i] = 1.0f;
    __syncthreads();

    // Equivalent to the HW kernel's `for j < SMEM_FLOATS / n`, but expressed
    // division-free: integer division by the RUNTIME divisor n compiles to a
    // software-divide routine that hangs the CDNA3 emulator. For n > 0,
    // (j+1)*n <= SMEM_FLOATS is identical to j < floor(SMEM_FLOATS / n), using
    // only a multiply + compare.
    float acc = 0.0f;

    for (int it = 0; it < inner_iters; ++it) {
        for (int j = 0; (j + 1) * n <= SMEM_FLOATS; ++j) {
            int idx = (lane * 32 + warp + j * nw) % SMEM_FLOATS;
            acc += smem[idx];   // read  — all lanes hit same bank
            smem[idx] = acc;    // write — all lanes hit same bank
        }
    }
    __syncthreads();

    if (tid == 0)
        d_sink[blockIdx.x] = acc;
}
