// GPU Benchmarks v1.1, tier2/polybench_mvt/hip/polybench_mvt.hip.
#include <hip/hip_runtime.h>
#include <cmath>

extern "C" __global__ void mvt_kernel1(const float* __restrict__ A,
                              const float* __restrict__ y1,
                              float* __restrict__ x1,
                              int n) {
    int i = blockDim.x * blockIdx.x + threadIdx.x;
    if (i >= n) return;
    float sum = 0.0f;
    for (int j = 0; j < n; j++)
        sum += A[i * n + j] * y1[j];
    x1[i] = sum;
}

extern "C" __global__ void mvt_kernel2(const float* __restrict__ A,
                              const float* __restrict__ y2,
                              float* __restrict__ x2,
                              int n) {
    int j = blockDim.x * blockIdx.x + threadIdx.x;
    if (j >= n) return;
    float sum = 0.0f;
    for (int i = 0; i < n; i++)
        sum += A[i * n + j] * y2[i];
    x2[j] = sum;
}
