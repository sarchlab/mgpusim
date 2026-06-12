#include <hip/hip_runtime.h>

// Branch divergence: 50% threads take each branch
// Even threads do multiply, odd threads do add
__global__ void branch_div_50pct_kernel(float* __restrict__ A,
                                        float* __restrict__ B,
                                        int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float val = A[gid];
        if (gid % 2 == 0) {
            // Even threads: multiply chain
            for (int i = 0; i < 512; i++) {
                val = val * 1.00001f + 0.00001f;
            }
        } else {
            // Odd threads: add chain
            for (int i = 0; i < 512; i++) {
                val = val + 0.00001f;
            }
        }
        B[gid] = val;
    }
}
