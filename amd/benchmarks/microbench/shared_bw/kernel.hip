#include <hip/hip_runtime.h>

#define BLOCK_SIZE 256

// Shared memory bandwidth: load from global to LDS, then store back
__global__ void shared_bw_kernel(const float* __restrict__ A,
                                 float* __restrict__ B,
                                 int N) {
    __shared__ float smem[BLOCK_SIZE];

    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    int lid = threadIdx.x;

    if (gid < N) {
        smem[lid] = A[gid];
    }
    __syncthreads();

    // Read and write through shared memory multiple times
    for (int i = 0; i < 128; i++) {
        smem[lid] = smem[lid] * 1.00001f + 0.00001f;
    }
    __syncthreads();

    if (gid < N) {
        B[gid] = smem[lid];
    }
}
