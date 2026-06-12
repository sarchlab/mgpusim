#include <hip/hip_runtime.h>

// DeviceMemory_Read: measures device memory read bandwidth.
// Each thread reads multiple elements from A and accumulates them.
__global__ void device_memory_read_kernel(const float* __restrict__ A,
                                          float* __restrict__ B,
                                          int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float sum = 0.0f;
        for (int stride = 0; stride < 16; stride++) {
            int idx = (gid + stride * (N / 16)) % N;
            sum += A[idx];
        }
        B[gid] = sum;
    }
}
