// GPU Benchmarks v1.1, tier2/polybench_atax/hip/polybench_atax.hip.
#include <hip/hip_runtime.h>

extern "C" __global__ void atax_kernel1(const float* __restrict__ A,
                              const float* __restrict__ x,
                              float* __restrict__ tmp,
                              int nx, int ny) {
    int i = blockDim.x * blockIdx.x + threadIdx.x;
    if (i < nx) {
        float sum = 0.0f;
        for (int j = 0; j < ny; j++)
            sum += A[i * ny + j] * x[j];
        tmp[i] = sum;
    }
}

extern "C" __global__ void atax_kernel2(const float* __restrict__ A,
                              const float* __restrict__ tmp,
                              float* __restrict__ y,
                              int nx, int ny) {
    int j = blockDim.x * blockIdx.x + threadIdx.x;
    if (j < ny) {
        float sum = 0.0f;
        for (int i = 0; i < nx; i++)
            sum += A[i * ny + j] * tmp[i];
        y[j] = sum;
    }
}
