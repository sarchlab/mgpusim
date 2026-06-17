/*
 * HIP kernel for Parboil 7-point 3D Jacobi stencil (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/parboil_stencil.
 *
 * 7-point stencil over an nx*ny*nz grid using ping-pong buffers. Each kernel
 * launch performs one time-step. Interior points (1-cell border fixed) are
 * updated:
 *   out[i] = c0*in[i] + c1*(in[i-1] + in[i+1] + in[i-nx] + in[i+nx]
 *                           + in[i-nx*ny] + in[i+nx*ny])
 *
 * NOTE: The original benchmark mapped the z-dimension onto blockIdx.z (a 3D
 * grid). The MGPUSim CDNA3 timing/functional models in this worktree only
 * honor the x/y grid dimensions for work-group dispatch (blockIdx.z reads as
 * 0). To stay within the supported subset, this port maps (ix, iy) onto a 2D
 * grid and loops the z-dimension inside the kernel. The computed result is
 * identical to the original 3D-grid version.
 *
 * Uses constant BLOCK_X/BLOCK_Y (16x16) for the block geometry so the
 * compiler emits no hidden ABI arguments (kernarg_segment_size = 36).
 */
#include "hip/hip_runtime.h"

#define BLOCK_X 16
#define BLOCK_Y 16

extern "C" __global__ void stencil3d(
    const float* __restrict__ in,
    float* __restrict__ out,
    int nx, int ny, int nz,
    float c0, float c1)
{
    int ix = blockIdx.x * BLOCK_X + threadIdx.x;
    int iy = blockIdx.y * BLOCK_Y + threadIdx.y;

    if (ix >= 1 && ix < nx - 1 &&
        iy >= 1 && iy < ny - 1) {
        for (int iz = 1; iz < nz - 1; ++iz) {
            int idx = iz * ny * nx + iy * nx + ix;
            out[idx] = c0 * in[idx]
                     + c1 * (in[idx - 1]
                           + in[idx + 1]
                           + in[idx - nx]
                           + in[idx + nx]
                           + in[idx - ny * nx]
                           + in[idx + ny * nx]);
        }
    }
}
