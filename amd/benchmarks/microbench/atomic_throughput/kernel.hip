#include <hip/hip_runtime.h>

__global__ void atomic_throughput_kernel(int *counters, int num_counters,
                                         int num_atomics_per_thread) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;

    for (int i = 0; i < num_atomics_per_thread; i++) {
        atomicAdd(&counters[tid % num_counters], 1);
    }
}
