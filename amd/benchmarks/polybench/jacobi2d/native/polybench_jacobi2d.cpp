// GPU Benchmarks v1.1, tier2/polybench_jacobi2d/hip/polybench_jacobi2d.hip.
#include <hip/hip_runtime.h>
#include <cmath>

extern "C" __global__ void jacobi2d_kernel(const float* a,float* b,int n) {
    const int i=blockIdx.y*blockDim.y+threadIdx.y+1;
    const int j=blockIdx.x*blockDim.x+threadIdx.x+1;
    if(i>=n-1 || j>=n-1)return;
    const int k=i*n+j;
    // Preserve the pin's center, left, right, down, up addition order.
    b[k]=0.2f*(a[k]+a[k-1]+a[k+1]+a[k+n]+a[k-n]);
}

extern "C" __global__ void jacobi2d_copy_kernel(const float* b,float* a,int n) {
    const int i=blockIdx.y*blockDim.y+threadIdx.y+1;
    const int j=blockIdx.x*blockDim.x+threadIdx.x+1;
    if(i<n-1 && j<n-1)a[i*n+j]=b[i*n+j];
}
