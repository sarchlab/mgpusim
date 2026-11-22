// GPU Benchmarks v1.1, tier2/polybench_bicg/hip/polybench_bicg.hip.
#include <hip/hip_runtime.h>

extern "C" __global__ void bicg_kernel1(const float* __restrict__ A,
                              const float* __restrict__ r,
                              float* __restrict__ s,
                              int m, int n) {
    int j = blockDim.x * blockIdx.x + threadIdx.x;
    if (j >= n) return;
    float sum = 0.0f;
    for (int i = 0; i < m; i++)
        sum += A[i * n + j] * r[i];
    s[j] = sum;
}

extern "C" __global__ void bicg_kernel2(const float* __restrict__ A,
                              const float* __restrict__ p,
                              float* __restrict__ q,
                              int m, int n) {
    int i = blockDim.x * blockIdx.x + threadIdx.x;
    if (i >= m) return;
    float sum = 0.0f;
    for (int j = 0; j < n; j++)
        sum += A[i * n + j] * p[j];
    q[i] = sum;
}
