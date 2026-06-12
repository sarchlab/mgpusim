#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

// Kernel 1: Update ey[0,j] from _fict_[t]
__global__ void fdtd2d_kernel1(DATA_TYPE *ey, DATA_TYPE *fict, int NY, int t) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (j < NY) {
        ey[0 * NY + j] = fict[t];
    }
}

// Kernel 2: Update ey[i,j] for i>0: ey[i][j] -= 0.5*(hz[i][j] - hz[i-1][j])
__global__ void fdtd2d_kernel2(DATA_TYPE *ey, DATA_TYPE *hz, int NX, int NY) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i >= 1 && i < NX && j < NY) {
        ey[i * NY + j] -= 0.5f * (hz[i * NY + j] - hz[(i - 1) * NY + j]);
    }
}

// Kernel 3: Update ex[i,j] for j>0: ex[i][j] -= 0.5*(hz[i][j] - hz[i][j-1])
__global__ void fdtd2d_kernel3(DATA_TYPE *ex, DATA_TYPE *hz, int NX, int NY) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NX && j >= 1 && j < NY) {
        ex[i * NY + j] -= 0.5f * (hz[i * NY + j] - hz[i * NY + (j - 1)]);
    }
}

// Kernel 4: Update hz: hz[i][j] -= 0.7*(ex[i][j+1]-ex[i][j] + ey[i+1][j]-ey[i][j])
__global__ void fdtd2d_kernel4(DATA_TYPE *ex, DATA_TYPE *ey, DATA_TYPE *hz,
                               int NX, int NY) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NX - 1 && j < NY - 1) {
        hz[i * NY + j] -= 0.7f * (ex[i * NY + (j + 1)] - ex[i * NY + j] +
                                   ey[(i + 1) * NY + j] - ey[i * NY + j]);
    }
}
