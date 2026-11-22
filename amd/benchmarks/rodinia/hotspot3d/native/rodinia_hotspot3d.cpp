// GPU Benchmarks v1.1, tier2/rodinia_hotspot3d/hip/rodinia_hotspot3d.hip.
// Retain the Rodinia source notice in UPSTREAM_LICENSE.
#include <hip/hip_runtime.h>

namespace hotspot3d {
struct Coefficients { float cap,rx,ry,rz,dt,upstream_dt,sdc,ce,cw,cn,cs,ct,cb,cc; };
}

extern "C" __global__ void hotspot3d_kernel(const float* __restrict__ src,float* __restrict__ dst,
                                 const float* __restrict__ power,int n,
                                 hotspot3d::Coefficients c,float ambient) {
    const int x=blockIdx.x*blockDim.x+threadIdx.x;
    const int y=blockIdx.y*blockDim.y+threadIdx.y;
    const int z=blockIdx.z*blockDim.z+threadIdx.z;
    if(x>=n || y>=n || z>=n)return;
    const int plane=n*n,i=z*plane+y*n+x;
    const int w=x?i-1:i,e=x+1<n?i+1:i;
    const int north=y?i-n:i,south=y+1<n?i+n:i;
    const int bottom=z?i-plane:i,top=z+1<n?i+plane:i;
    // Preserve the pinned hotspotOpt1 GPU sum order and all six clamped faces.
    dst[i]=c.cc*src[i]+c.cw*src[w]+c.ce*src[e]+c.cs*src[south]+c.cn*src[north]
           +c.cb*src[bottom]+c.ct*src[top]+c.sdc*power[i]+c.ct*ambient;
}
