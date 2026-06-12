#include <hip/hip_runtime.h>

// DeviceMemory_Write: measures device memory write bandwidth.
// Each thread writes to multiple locations in B.
__global__ void device_memory_write_kernel(float* __restrict__ B,
                                           int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        B[gid] = (float)gid;
    }
}
