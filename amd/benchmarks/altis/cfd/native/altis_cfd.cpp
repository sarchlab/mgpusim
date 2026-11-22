// GPU Benchmarks v1.1, tier2/altis_cfd/hip/altis_cfd.hip.
#include <hip/hip_runtime.h>
#include "cfd_kernels.h"

extern "C" __global__ void copy_variables_kernel(int n,const float* state,float* old) {
    const int i=blockIdx.x*blockDim.x+threadIdx.x;
    for(int k=0;k<5;++k)old[i+k*n]=state[i+k*n];
}
