/*
 * HIP kernel for Parboil CUTCP (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/parboil_cutcp.
 *
 * Coulombic potential with cutoff: direct summation of electrostatic
 * charge interactions on a 3D grid. Each thread computes the potential at
 * one grid point by iterating over all atoms within a cutoff radius.
 *
 * A constant BLOCK_SIZE (not blockDim.x) is used for the 1D launch so the
 * compiler emits no hidden ABI arguments (smaller, simpler kernarg).
 * Atoms are passed as a flat float4 array {x, y, z, charge}.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 128

extern "C" __global__ void cutcp_kernel(
    const float4* __restrict__ atoms,
    float*        __restrict__ potential,
    int           num_atoms,
    int           grid_side,
    float         grid_spacing,
    float         cutoff2)
{
    int gid = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int total_points = grid_side * grid_side * grid_side;
    if (gid >= total_points) return;

    int gz = gid / (grid_side * grid_side);
    int gy = (gid / grid_side) % grid_side;
    int gx = gid % grid_side;

    float px = gx * grid_spacing;
    float py = gy * grid_spacing;
    float pz = gz * grid_spacing;

    float pot = 0.0f;

    for (int i = 0; i < num_atoms; ++i) {
        float4 atom = atoms[i];
        float dx = px - atom.x;
        float dy = py - atom.y;
        float dz = pz - atom.z;
        float r2 = dx * dx + dy * dy + dz * dz;

        if (r2 < cutoff2 && r2 > 1e-12f) {
            float r = sqrtf(r2);
            pot += atom.w / r;
        }
    }

    potential[gid] = pot;
}
