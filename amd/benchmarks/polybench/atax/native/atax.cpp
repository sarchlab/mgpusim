/**
 * atax.cpp: HIP implementation of PolyBench/GPU ATAX kernels
 * Translated from atax.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

typedef float DATA_TYPE;

extern "C" __global__ void atax_kernel1(DATA_TYPE *A, DATA_TYPE *x, DATA_TYPE *tmp, int nx, int ny) {
    
    int i = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    if (i < nx)
    {
        int j;
        for(j=0; j < ny; j++)
        {
            tmp[i] += A[i * ny + j] * x[j];
        }
    }
}

extern "C" __global__ void atax_kernel2(DATA_TYPE *A, DATA_TYPE *y, DATA_TYPE *tmp, int nx, int ny) {
    
    int j = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    if (j < ny)
    {
        int i;
        for(i=0; i < nx; i++)
        {
            y[j] += A[i * ny + j] * tmp[i];
        }
    }
}
