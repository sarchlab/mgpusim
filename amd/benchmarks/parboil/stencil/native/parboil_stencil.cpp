#include <hip/hip_runtime.h>

// GPU Benchmarks v1.1, tier2/parboil_stencil/hip/parboil_stencil.hip.
extern "C" __global__ void stencil3d(const float* __restrict__ in,
                          float*       __restrict__ out,
                          int nx, int ny, int nz,
                          float c0, float c1) {
    int ix = blockIdx.x * blockDim.x + threadIdx.x;
    int iy = blockIdx.y * blockDim.y + threadIdx.y;
    int iz = blockIdx.z;

    // Only compute interior points (skip 1-cell boundary)
    if (ix >= 1 && ix < nx - 1 &&
        iy >= 1 && iy < ny - 1 &&
        iz >= 1 && iz < nz - 1) {
        int idx = iz * ny * nx + iy * nx + ix;
        // Preserve the pinned cuda_base expression and neighbor order.
        out[idx] = (in[idx + ny*nx] + in[idx - ny*nx]
                  + in[idx + nx] + in[idx - nx]
                  + in[idx + 1] + in[idx - 1]) * c1 - in[idx] * c0;
    }
}
