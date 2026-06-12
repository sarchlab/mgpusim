#include <hip/hip_runtime.h>

// Fused SwiGLU: y = (x * sigmoid(x)) * gate = silu(x) * gate
__global__ void fused_swiglu_kernel(const float* __restrict__ A,
                                    const float* __restrict__ gate,
                                    float* __restrict__ B,
                                    int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float x = A[gid];
        float g = gate[gid];
        float sig = 1.0f / (1.0f + expf(-x));
        B[gid] = x * sig * g;
    }
}
