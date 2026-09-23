/*
 * HIP kernel for NAS Parallel Benchmarks - Embarrassingly Parallel (EP)
 * (gfx942 / CDNA3). Extracted from sarchlab/gpu_benchmarks tier2/npb_ep.
 *
 * Each thread generates one pair of uniform random deviates with a per-thread
 * integer LCG, applies the Box-Muller transform, and computes an annular bin
 * index. The original benchmark atomically increments global bin counters;
 * the CDNA3 functional emulator does not implement global/FLAT atomics, so
 * here each thread instead writes its computed bin index to a per-thread
 * output array (binOut[idx]). The host (Verify) bins the results, reproducing
 * the same counts. The dominant compute (LCG + Box-Muller + bin) is identical.
 *
 * Uses a constant BLOCK_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments.
 */
#include "hip/hip_runtime.h"

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

#define NUM_BINS   10
#define BLOCK_SIZE 256

// ANSI C LCG, mod 2^32 by unsigned overflow.
__device__ static inline unsigned int lcg_next(unsigned int seed) {
    return seed * 1103515245u + 12345u;
}

extern "C" __global__ void ep_kernel(int N, int* __restrict__ binOut) {
    int idx = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (idx >= N) return;

    unsigned int seed = (unsigned int)(idx + 1);
    seed = lcg_next(seed);
    seed = lcg_next(seed);

    seed = lcg_next(seed);
    float u1 = (float)seed / 4294967296.0f;
    seed = lcg_next(seed);
    float u2 = (float)seed / 4294967296.0f;

    if (u1 < 1e-10f) u1 = 1e-10f;

    float r = sqrtf(-2.0f * logf(u1));
    float theta = 2.0f * (float)M_PI * u2;
    float x1 = r * cosf(theta);
    float x2 = r * sinf(theta);

    float t = x1 * x1 + x2 * x2;

    int bin = (int)sqrtf(t);
    if (bin >= NUM_BINS) bin = NUM_BINS - 1;
    if (bin < 0) bin = 0;

    binOut[idx] = bin;
}
