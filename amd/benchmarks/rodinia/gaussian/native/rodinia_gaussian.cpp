// GPU Benchmarks v1.1, tier2/rodinia_gaussian/hip/rodinia_gaussian.hip.
#include <hip/hip_runtime.h>

extern "C" __global__ void fan1(float* __restrict__ m,
                     const float* __restrict__ a,
                     int Size, int t)
{
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid < Size - 1 - t) {
        int row = t + 1 + tid;
        m[row * Size + t] = a[row * Size + t] / a[t * Size + t];
    }
}

extern "C" __global__ void fan2(const float* __restrict__ m,
                     float* __restrict__ a,
                     float* __restrict__ b,
                     int Size, int t)
{
    int col = blockIdx.x * blockDim.x + threadIdx.x;  // relative column index
    int row = blockIdx.y * blockDim.y + threadIdx.y;  // relative row index
    int remaining = Size - t - 1;

    if (col < remaining && row < remaining) {
        int abs_row = t + 1 + row;
        int abs_col = t + 1 + col;
        a[abs_row * Size + abs_col] -= m[abs_row * Size + t] * a[t * Size + abs_col];
        if (col == 0) {
            b[abs_row] -= m[abs_row * Size + t] * b[t];
        }
    }
}
