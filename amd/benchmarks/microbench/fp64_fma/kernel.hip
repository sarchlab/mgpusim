#include <hip/hip_runtime.h>

__global__ void fp64_fma_kernel(int num_ops, double *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    double a = (double)(tid + 1) * 1.00000000001;
    double b = 1.00000000001;
    double c = 0.99999999999;

    for (int i = 0; i < num_ops; i++) {
        a = a * b + c;
        b = b * a + c;
        a = a * c + b;
        b = b * c + a;
        asm volatile("" : "+v"(a), "+v"(b));
    }

    d_out[tid] = a + b;
}
