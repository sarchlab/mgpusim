// Device source from GPU Benchmarks v1.1 (a8ca84f8), shared_mem_bandwidth.hip.
#include <hip/hip_runtime.h>
#include "shared_mem_pattern.h"

template <bool Conflict>
__device__ __forceinline__ void smem_bw_pattern(float *d_sink, int inner_iters)
{
    // Volatile preserves the requested shared loads/stores in the compiled loop.
    __shared__ volatile float smem[SMEM_FLOATS];
    const int tid = threadIdx.x;
    const int n = blockDim.x;

    for (int i = tid; i < SMEM_FLOATS; i += n) smem[i] = 1.0f;
    __syncthreads();

    float acc = 0.0f;
    for (int it = 0; it < inner_iters; ++it) {
        for (int i = tid; i < SMEM_FLOATS; i += n) {
            const int index = Conflict ? smem_conflict_index(i) : i;
            // A bounded dependent recurrence: acc and every element stay in
            // [0,1]. The former running sum overflowed on the declared sweep.
            acc = 0.5f * (acc + smem[index]);
            smem[index] = acc;
        }
    }
    __syncthreads();

    const size_t global = static_cast<size_t>(blockIdx.x) * blockDim.x + tid;
    d_sink[global] = acc;
}

extern "C" __global__ void smem_bw_no_conflict(float *d_sink, int inner_iters)
{
    smem_bw_pattern<false>(d_sink, inner_iters);
}

extern "C" __global__ void smem_bw_conflict(float *d_sink, int inner_iters)
{
    smem_bw_pattern<true>(d_sink, inner_iters);
}
