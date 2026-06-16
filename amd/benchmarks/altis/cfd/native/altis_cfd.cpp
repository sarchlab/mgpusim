/*
 * HIP kernel for Altis CFD (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/altis_cfd.
 *
 * Compressible-Euler flux computation on a synthetic unstructured mesh
 * using a Rusanov (local Lax-Friedrichs) finite-volume scheme. Each
 * thread processes one cell and accumulates flux contributions from its
 * NUM_NEIGHBORS face neighbors.
 *
 * A constant BLOCK_SIZE is used for the block geometry (instead of
 * blockDim.x) so the compiler emits no hidden ABI arguments.
 */
#include "hip/hip_runtime.h"

#define GAMMA 1.4f
#define GAMMA_M1 0.4f
#define NUM_NEIGHBORS 4
#define BLOCK_SIZE 256

__device__ static inline float compute_pressure(float rho, float mx, float my,
                                                float mz, float e) {
    float ke = 0.5f * (mx * mx + my * my + mz * mz) / fmaxf(rho, 1e-10f);
    return GAMMA_M1 * (e - ke);
}

__device__ static inline float compute_speed_of_sound(float rho, float p) {
    return sqrtf(fmaxf(GAMMA * p / fmaxf(rho, 1e-10f), 1e-10f));
}

extern "C" __global__ void compute_flux_kernel(
    const float* __restrict__ rho,
    const float* __restrict__ mx,
    const float* __restrict__ my,
    const float* __restrict__ mz,
    const float* __restrict__ energy,
    const int*   __restrict__ neighbors,  // [N * NUM_NEIGHBORS]
    const float* __restrict__ normals,    // [N * NUM_NEIGHBORS * 3]
    const float* __restrict__ areas,      // [N * NUM_NEIGHBORS]
    float* __restrict__ flux_rho,
    float* __restrict__ flux_mx,
    float* __restrict__ flux_my,
    float* __restrict__ flux_mz,
    float* __restrict__ flux_energy,
    int N)
{
    int idx = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (idx >= N) return;

    float rho_i = rho[idx];
    float mx_i  = mx[idx];
    float my_i  = my[idx];
    float mz_i  = mz[idx];
    float e_i   = energy[idx];
    float p_i   = compute_pressure(rho_i, mx_i, my_i, mz_i, e_i);
    float a_i   = compute_speed_of_sound(rho_i, p_i);

    float inv_rho_i = 1.0f / fmaxf(rho_i, 1e-10f);
    float vx_i = mx_i * inv_rho_i;
    float vy_i = my_i * inv_rho_i;
    float vz_i = mz_i * inv_rho_i;

    float f_rho = 0.0f, f_mx = 0.0f, f_my = 0.0f, f_mz = 0.0f, f_e = 0.0f;

    for (int f = 0; f < NUM_NEIGHBORS; ++f) {
        int j = neighbors[idx * NUM_NEIGHBORS + f];
        int nbase = (idx * NUM_NEIGHBORS + f) * 3;
        float nx = normals[nbase + 0];
        float ny = normals[nbase + 1];
        float nz = normals[nbase + 2];
        float area = areas[idx * NUM_NEIGHBORS + f];

        float rho_j = rho[j];
        float mx_j  = mx[j];
        float my_j  = my[j];
        float mz_j  = mz[j];
        float e_j   = energy[j];
        float p_j   = compute_pressure(rho_j, mx_j, my_j, mz_j, e_j);
        float a_j   = compute_speed_of_sound(rho_j, p_j);

        float inv_rho_j = 1.0f / fmaxf(rho_j, 1e-10f);
        float vx_j = mx_j * inv_rho_j;
        float vy_j = my_j * inv_rho_j;
        float vz_j = mz_j * inv_rho_j;

        float vn_i = vx_i * nx + vy_i * ny + vz_i * nz;
        float vn_j = vx_j * nx + vy_j * ny + vz_j * nz;

        float f_rho_i = rho_i * vn_i;
        float f_rho_j = rho_j * vn_j;

        float f_mx_i = mx_i * vn_i + p_i * nx;
        float f_mx_j = mx_j * vn_j + p_j * nx;

        float f_my_i = my_i * vn_i + p_i * ny;
        float f_my_j = my_j * vn_j + p_j * ny;

        float f_mz_i = mz_i * vn_i + p_i * nz;
        float f_mz_j = mz_j * vn_j + p_j * nz;

        float f_e_i = (e_i + p_i) * vn_i;
        float f_e_j = (e_j + p_j) * vn_j;

        float lambda = fmaxf(fabsf(vn_i) + a_i, fabsf(vn_j) + a_j);

        f_rho += area * (0.5f * (f_rho_i + f_rho_j) - 0.5f * lambda * (rho_j - rho_i));
        f_mx  += area * (0.5f * (f_mx_i  + f_mx_j)  - 0.5f * lambda * (mx_j  - mx_i));
        f_my  += area * (0.5f * (f_my_i  + f_my_j)  - 0.5f * lambda * (my_j  - my_i));
        f_mz  += area * (0.5f * (f_mz_i  + f_mz_j)  - 0.5f * lambda * (mz_j  - mz_i));
        f_e   += area * (0.5f * (f_e_i   + f_e_j)   - 0.5f * lambda * (e_j   - e_i));
    }

    flux_rho[idx]    = f_rho;
    flux_mx[idx]     = f_mx;
    flux_my[idx]     = f_my;
    flux_mz[idx]     = f_mz;
    flux_energy[idx] = f_e;
}
