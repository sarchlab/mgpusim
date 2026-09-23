/*
 * HIP kernel for the cache_latency microbenchmark (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier1/cache_latency.
 *
 * Single-thread pointer-chasing latency measurement: one thread walks a
 * linked-list-style index chain where each array element holds the index of
 * the next element to visit. Because each load depends on the previous result
 * (a true data dependency), the loads cannot overlap, directly exposing the
 * per-access memory latency rather than bandwidth.
 *
 * Launched with a single work-item (grid 1, block 1), matching the HIP source.
 * The kernel reads blockIdx.x / threadIdx.x only to guard against extra
 * work-items; it uses a constant launch geometry, so no hidden ABI arguments
 * are emitted (kernarg_segment_size = 24).
 */
#include "hip/hip_runtime.h"
#include <cstdint>

extern "C" __global__ void pointer_chase_kernel(
    const uint32_t* __restrict__ arr,
    uint32_t start_idx,
    uint32_t num_accesses,
    uint32_t* result)
{
    if (blockIdx.x != 0 || threadIdx.x != 0) return;

    uint32_t idx = start_idx;

    for (uint32_t i = 0; i < num_accesses; ++i) {
        idx = arr[idx];
    }

    *result = idx;
}
