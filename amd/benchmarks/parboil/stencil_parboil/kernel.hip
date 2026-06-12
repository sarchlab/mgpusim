#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

// 7-point 3D stencil kernel (Jacobi iteration)
// Updates interior points only; boundary cells are left unchanged.
extern "C" __global__ void stencil_kernel(DATA_TYPE *in, DATA_TYPE *out,
                                           int nx, int ny, int nz,
                                           DATA_TYPE coeff_center,
                                           DATA_TYPE coeff_neighbor) {
    int ix = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int iy = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;
    int iz = hipBlockIdx_z;

    // Only compute interior points (skip boundaries)
    if (ix >= 1 && ix < nx - 1 &&
        iy >= 1 && iy < ny - 1 &&
        iz >= 1 && iz < nz - 1) {
        int idx = iz * ny * nx + iy * nx + ix;
        out[idx] = coeff_center * in[idx] +
                   coeff_neighbor * (in[idx - 1] +        // x-1
                                     in[idx + 1] +        // x+1
                                     in[idx - nx] +       // y-1
                                     in[idx + nx] +       // y+1
                                     in[idx - ny * nx] +  // z-1
                                     in[idx + ny * nx]);  // z+1
    }
}
