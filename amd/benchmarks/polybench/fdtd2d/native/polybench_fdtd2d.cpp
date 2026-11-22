// GPU Benchmarks v1.1, tier2/polybench_fdtd2d/hip/polybench_fdtd2d.hip.
#include <hip/hip_runtime.h>
#include <cmath>

extern "C" __global__ void fdtd_update_ex(float* __restrict__ ex,
                                const float* __restrict__ hz,
                                int NX, int NY) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i >= NX || j >= NY) return;

    if (j >= 1) {
        ex[i * NY + j] -= 0.5f * (hz[i * NY + j] - hz[i * NY + (j - 1)]);
    }
}

extern "C" __global__ void fdtd_update_ey(float* __restrict__ ey,
                                const float* __restrict__ hz,
                                int NX, int NY, float fict_t) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i >= NX || j >= NY) return;

    if (i == 0) {
        ey[0 * NY + j] = fict_t;
    } else {
        ey[i * NY + j] -= 0.5f * (hz[i * NY + j] - hz[(i - 1) * NY + j]);
    }
}

extern "C" __global__ void fdtd_update_hz(const float* __restrict__ ex,
                                const float* __restrict__ ey,
                                float* __restrict__ hz,
                                int NX, int NY) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i >= NX - 1 || j >= NY - 1) return;

    hz[i * NY + j] -= 0.7f * (ex[i * NY + (j + 1)] - ex[i * NY + j] +
                               ey[(i + 1) * NY + j] - ey[i * NY + j]);
}
