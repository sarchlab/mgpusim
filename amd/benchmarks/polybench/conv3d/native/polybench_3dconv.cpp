// GPU Benchmarks v1.1, tier2/polybench_3dconv/hip/polybench_3dconv.hip.
#include <hip/hip_runtime.h>
#include <cmath>

extern "C" __global__ void conv3d_kernel(const float* __restrict__ input,
                             float* __restrict__ output, int N) {
    int k = blockIdx.x * blockDim.x + threadIdx.x;
    int j = blockIdx.y * blockDim.y + threadIdx.y;
    int i = blockIdx.z * blockDim.z + threadIdx.z;
    if (i < 1 || j < 1 || k < 1 || i >= N-1 || j >= N-1 || k >= N-1) return;
    const int plane = N*N;
    // Preserve all fifteen terms and their order, including repeated offsets.
    output[i*plane+j*N+k] =
          2.0f * input[(i-1)*plane+(j-1)*N+k-1]
        + 4.0f * input[(i+1)*plane+(j-1)*N+k-1]
        + 5.0f * input[(i-1)*plane+(j-1)*N+k-1]
        + 7.0f * input[(i+1)*plane+(j-1)*N+k-1]
        - 8.0f * input[(i-1)*plane+(j-1)*N+k-1]
        +10.0f * input[(i+1)*plane+(j-1)*N+k-1]
        - 3.0f * input[i*plane+(j-1)*N+k]
        + 6.0f * input[i*plane+j*N+k]
        - 9.0f * input[i*plane+(j+1)*N+k]
        + 2.0f * input[(i-1)*plane+(j-1)*N+k+1]
        + 4.0f * input[(i+1)*plane+(j-1)*N+k+1]
        + 5.0f * input[(i-1)*plane+j*N+k+1]
        + 7.0f * input[(i+1)*plane+j*N+k+1]
        - 8.0f * input[(i-1)*plane+(j+1)*N+k+1]
        +10.0f * input[(i+1)*plane+(j+1)*N+k+1];
}
