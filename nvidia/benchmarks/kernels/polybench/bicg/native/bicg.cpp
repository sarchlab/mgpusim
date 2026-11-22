/**
 * bicg.cpp: HIP implementation of PolyBench/GPU BICG kernels
 * Translated from bicg.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

typedef float DATA_TYPE;

extern "C" __global__ void bicgKernel1(DATA_TYPE *A, DATA_TYPE *p, DATA_TYPE *q, int nx, int ny) 
{
    int i = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    
    if (i < nx)
    {
        q[i] = 0.0;

        int j;
        for(j=0; j < ny; j++)
        {
            q[i] += A[i * ny + j] * p[j];
        }
    }
    
}

extern "C" __global__ void bicgKernel2(DATA_TYPE *A, DATA_TYPE *r, DATA_TYPE *s, int nx, int ny) 
{
    int j = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    
    if (j < ny)
    {
        s[j] = 0.0;

        int i;
        for(i = 0; i < nx; i++)
        {
            s[j] += A[i * ny + j] * r[i];
        }
    }
    
}
