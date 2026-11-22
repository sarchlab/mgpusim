/*
 * HIP kernel for Rodinia LavaMD (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/rodinia_lavamd.
 *
 * Short-range molecular dynamics with cell-list decomposition. Computes
 * Lennard-Jones type particle interactions within neighboring cells
 * (up to 27 cells per box). One workgroup processes one box.
 *
 * Uses a constant BLOCK_SIZE (not blockDim) for the loop stride so the
 * compiler emits no hidden ABI arguments (smaller, simpler kernarg).
 */
#include "hip/hip_runtime.h"

#define LJ_A   2.0f
#define LJ_B   1.0f
#define BLOCK_SIZE 128

extern "C" __global__ void lavamd_kernel(
    const float* __restrict__ pos_x,
    const float* __restrict__ pos_y,
    const float* __restrict__ pos_z,
    float* __restrict__ force_x,
    float* __restrict__ force_y,
    float* __restrict__ force_z,
    float* __restrict__ energy_out,
    const int*   __restrict__ neighbor_list,
    const int*   __restrict__ neighbor_count,
    int particles_per_box,
    int total_boxes)
{
    int box_id = blockIdx.x;
    if (box_id >= total_boxes) return;

    int tid = threadIdx.x;
    int base_i = box_id * particles_per_box;

    for (int p = tid; p < particles_per_box; p += BLOCK_SIZE) {
        int i = base_i + p;
        float px = pos_x[i];
        float py = pos_y[i];
        float pz = pos_z[i];

        float fx = 0.0f, fy = 0.0f, fz = 0.0f;
        float pe = 0.0f;

        int n_neighbors = neighbor_count[box_id];

        for (int n = 0; n < n_neighbors; ++n) {
            int nbox = neighbor_list[box_id * 27 + n];
            int base_j = nbox * particles_per_box;

            for (int q = 0; q < particles_per_box; ++q) {
                int j = base_j + q;

                float dx = px - pos_x[j];
                float dy = py - pos_y[j];
                float dz = pz - pos_z[j];

                float r2 = dx * dx + dy * dy + dz * dz;

                if (r2 > 1e-10f) {
                    float r2inv = 1.0f / r2;
                    float r6inv = r2inv * r2inv * r2inv;

                    float force = r2inv * r6inv * (LJ_A * r6inv - LJ_B);
                    pe += r6inv * (LJ_A * r6inv - LJ_B);

                    fx += force * dx;
                    fy += force * dy;
                    fz += force * dz;
                }
            }
        }

        force_x[i]    = fx;
        force_y[i]    = fy;
        force_z[i]    = fz;
        energy_out[i] = pe;
    }
}
