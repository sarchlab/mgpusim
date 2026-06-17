/*
 * HIP kernel for Rodinia HotSpot3D (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/rodinia_hotspot3d.
 *
 * 3D stencil thermal simulation. Each cell's temperature is updated from its
 * six neighbors (+/-x, +/-y, +/-z), local power density, and thermal
 * resistances in an NxNxN grid. Boundaries are clamped (a cell at the edge
 * uses its own value for the missing neighbor).
 *
 * The launch geometry uses a constant BLOCK_DIM (8) square for the (x, y)
 * block, so the compiler emits no hidden ABI arguments. Each work-item owns
 * one (x, y) column and walks the full z range in a loop, so the kernel never
 * references blockIdx.z / threadIdx.z. (The MGPUSim gfx942 model does not wire
 * up the work-group ID Z system SGPR, so a 3D grid would read blockIdx.z as 0;
 * folding z into an in-kernel loop sidesteps that and is numerically
 * identical.) Grid x/y are ceil(N / BLOCK_DIM) each; the in-kernel bounds
 * checks against nx/ny mask off any out-of-range work-items.
 */
#include "hip/hip_runtime.h"

#define BLOCK_DIM 8

extern "C" __global__ void hotspot3d_kernel(
    const float* __restrict__ temp_src,
    float* __restrict__ temp_dst,
    const float* __restrict__ power,
    int nx, int ny, int nz,
    float step_div_cap,
    float Rx_1, float Ry_1, float Rz_1,
    float Ra_1, float amb_temp)
{
    int x = blockIdx.x * BLOCK_DIM + threadIdx.x;
    int y = blockIdx.y * BLOCK_DIM + threadIdx.y;

    if (x >= nx || y >= ny) return;

    int plane = ny * nx;

    for (int z = 0; z < nz; ++z) {
        int idx = z * plane + y * nx + x;

        float tc = temp_src[idx];

        // +/-x neighbors (clamped boundary)
        float txm = (x > 0)      ? temp_src[z * plane + y * nx + (x - 1)] : tc;
        float txp = (x < nx - 1) ? temp_src[z * plane + y * nx + (x + 1)] : tc;

        // +/-y neighbors (clamped boundary)
        float tym = (y > 0)      ? temp_src[z * plane + (y - 1) * nx + x] : tc;
        float typ = (y < ny - 1) ? temp_src[z * plane + (y + 1) * nx + x] : tc;

        // +/-z neighbors (clamped boundary)
        float tzm = (z > 0)      ? temp_src[(z - 1) * plane + y * nx + x] : tc;
        float tzp = (z < nz - 1) ? temp_src[(z + 1) * plane + y * nx + x] : tc;

        float delta = step_div_cap * (
            power[idx]
            + (txm + txp - 2.0f * tc) * Rx_1
            + (tym + typ - 2.0f * tc) * Ry_1
            + (tzm + tzp - 2.0f * tc) * Rz_1
            + (amb_temp - tc) * Ra_1
        );

        temp_dst[idx] = tc + delta;
    }
}
