#include <hip/hip_runtime.h>

// BusSpeedDownload: read from device buffer (measures H2D transfer effect)
// Each thread reads one float from A and writes to B.
__global__ void bus_speed_download_kernel(const float* __restrict__ A,
                                          float* __restrict__ B,
                                          int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        B[gid] = A[gid];
    }
}
