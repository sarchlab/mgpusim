#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

// Fan1: Divide pivot row elements by the pivot element
__global__ void Fan1(DATA_TYPE *M, DATA_TYPE *a, int size, int t) {
    int idx = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (idx < size - 1 - t) {
        M[(size) * (t + 1 + idx) + t] =
            a[(size) * (t + 1 + idx) + t] / a[size * t + t];
    }
}

// Fan2: Eliminate elements below the pivot
__global__ void Fan2(DATA_TYPE *M, DATA_TYPE *a, DATA_TYPE *b, int size,
                     int t) {
    int xidx = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int yidx = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (xidx < size - 1 - t && yidx < size - t) {
        int row = t + 1 + xidx;
        int col = t + 1 + yidx;
        a[size * row + col] -= M[size * row + t] * a[size * t + col];
        if (yidx == 0) {
            b[row] -= M[size * row + t] * b[t];
        }
    }
}
