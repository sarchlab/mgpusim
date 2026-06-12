#include <hip/hip_runtime.h>

__global__ void fp32_fma_kernel(int num_ops, float *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    float a = (float)(tid + 1) * 1.0001f;
    float b = 1.0001f;
    float c = 0.9999f;

    for (int i = 0; i < num_ops; i++) {
        a = a * b + c;
        b = b * a + c;
        a = a * c + b;
        b = b * c + a;
        asm volatile("" : "+v"(a), "+v"(b));
    }

    d_out[tid] = a + b;
}
