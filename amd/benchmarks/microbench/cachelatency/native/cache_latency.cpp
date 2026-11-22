// GPU Benchmarks v1.1, tier1/cache_latency/hip/cache_latency.hip.
#include <hip/hip_runtime.h>
#include <stdint.h>

extern "C" __global__ void pointer_chase_kernel(
    const uint32_t * __restrict__ arr,
    uint32_t  start_idx,
    uint64_t  num_accesses,
    uint32_t *result)
{
    if (blockIdx.x != 0 || threadIdx.x != 0) return;

    uint32_t idx = start_idx;

    for (uint64_t i = 0; i < num_accesses; ++i) {
        idx = arr[idx];
    }

    *result = idx;
}
