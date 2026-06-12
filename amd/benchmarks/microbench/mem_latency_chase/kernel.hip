#include <hip/hip_runtime.h>

// Memory latency: pointer chase through array
// Indices[] contains a permutation so Indices[i] points to the next element
__global__ void mem_latency_chase_kernel(const int* __restrict__ Indices,
                                         int* __restrict__ Result,
                                         int N,
                                         int numChases) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        int idx = gid % N;
        for (int i = 0; i < numChases; i++) {
            idx = Indices[idx];
        }
        Result[gid] = idx;
    }
}
