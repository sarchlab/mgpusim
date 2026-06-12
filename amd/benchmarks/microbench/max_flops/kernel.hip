#include <hip/hip_runtime.h>

// MaxFlops: maximizes floating-point throughput with FMA chains.
// Each thread performs a long chain of FMA operations.
__global__ void max_flops_kernel(float* __restrict__ A,
                                 float* __restrict__ B,
                                 int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float a = A[gid];
        float b = 1.00001f;
        float c = 0.00001f;
        for (int i = 0; i < 2048; i++) {
            a = a * b + c;
        }
        B[gid] = a;
    }
}
