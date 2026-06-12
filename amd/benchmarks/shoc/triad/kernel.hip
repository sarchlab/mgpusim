#include <hip/hip_runtime.h>

// ---------------------------------------------------------------------------
// Kernel: Stream triad a[i] = b[i] + scalar * c[i]
// ---------------------------------------------------------------------------
__global__ void triad_kernel(float* __restrict__ a,
                             const float* __restrict__ b,
                             const float* __restrict__ c,
                             float scalar, int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < N) {
        a[i] = b[i] + scalar * c[i];
    }
}
