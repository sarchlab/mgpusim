#include <hip/hip_runtime.h>

__global__ void sfun_sin_kernel(int num_ops, float *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    float val = (float)(tid + 1) * 0.0001f;

    for (int i = 0; i < num_ops; i++) {
        val = sinf(val);
        val = sinf(val);
        val = sinf(val);
        val = sinf(val);
        asm volatile("" : "+v"(val));
    }

    d_out[tid] = val;
}
