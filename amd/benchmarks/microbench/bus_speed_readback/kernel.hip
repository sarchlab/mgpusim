#include <hip/hip_runtime.h>

// BusSpeedReadback: write to device buffer (measures D2H transfer effect)
// Each thread reads from A and writes to B.
__global__ void bus_speed_readback_kernel(const float* __restrict__ A,
                                          float* __restrict__ B,
                                          int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        B[gid] = A[gid];
    }
}
