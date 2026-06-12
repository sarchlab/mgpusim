#include <hip/hip_runtime.h>

__global__ void int_mad_kernel(int num_ops, int *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    int a = tid + 1;
    int b = 3;
    int c = 7;

    for (int i = 0; i < num_ops; i++) {
        a = a * b + c;
        b = b * a + c;
        a = a * c + b;
        b = b * c + a;
        asm volatile("" : "+v"(a), "+v"(b));
    }

    d_out[tid] = a + b;
}
