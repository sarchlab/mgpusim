#include <hip/hip_runtime.h>

// Global memory bandwidth: copy array A to array B
__global__ void global_bw_copy_kernel(const float* __restrict__ A,
                                      float* __restrict__ B,
                                      int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        B[gid] = A[gid];
    }
}
