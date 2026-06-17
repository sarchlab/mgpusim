/*
 * HIP kernel for Parboil LBM (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/parboil_lbm.
 *
 * Lattice Boltzmann Method (D3Q19): fused collide-stream kernel applying the
 * BGK collision operator and streaming to neighbor cells, with bounce-back
 * boundary conditions at grid boundaries.
 *
 * The D3Q19 velocity set, weights and opposite-direction tables are expressed
 * via constexpr lookup functions over a fully-unrolled loop, so the compiler
 * materializes every table entry as an immediate operand. This avoids any
 * runtime constant-memory load (which the MGPUSim CDNA3 functional emulator
 * does not map) and the wide scalar loads its timing scalar unit does not
 * model.
 *
 * A constant BLOCK_SIZE is used instead of blockDim.x for the linear index so
 * the compiler emits no hidden ABI arguments (kernarg_segment_size = 32).
 */
#include "hip/hip_runtime.h"

#define Q 19
#define BLOCK_SIZE 128

__device__ __forceinline__ int lbm_ex(int q) {
    constexpr int t[Q] = { 0, 1,-1, 0, 0, 0, 0, 1,-1, 1,-1, 1,-1, 1,-1, 0, 0, 0, 0};
    return t[q];
}
__device__ __forceinline__ int lbm_ey(int q) {
    constexpr int t[Q] = { 0, 0, 0, 1,-1, 0, 0, 1, 1,-1,-1, 0, 0, 0, 0, 1,-1, 1,-1};
    return t[q];
}
__device__ __forceinline__ int lbm_ez(int q) {
    constexpr int t[Q] = { 0, 0, 0, 0, 0, 1,-1, 0, 0, 0, 0, 1, 1,-1,-1, 1, 1,-1,-1};
    return t[q];
}
__device__ __forceinline__ float lbm_w(int q) {
    constexpr float t[Q] = {
        1.0f/3.0f,
        1.0f/18.0f, 1.0f/18.0f, 1.0f/18.0f,
        1.0f/18.0f, 1.0f/18.0f, 1.0f/18.0f,
        1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f,
        1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f,
        1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f
    };
    return t[q];
}
__device__ __forceinline__ int lbm_opp(int q) {
    constexpr int t[Q] = {0, 2,1, 4,3, 6,5, 10,9,8,7, 14,13,12,11, 18,17,16,15};
    return t[q];
}

extern "C" __global__ void lbm_collide_stream_kernel(
    const float* __restrict__ f_src,
    float*       __restrict__ f_dst,
    int Nx, int Ny, int Nz,
    float omega)
{
    int idx = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int N = Nx * Ny * Nz;
    if (idx >= N) return;

    int iz = idx / (Nx * Ny);
    int iy = (idx / Nx) % Ny;
    int ix = idx % Nx;

    bool is_boundary = (ix == 0 || ix == Nx-1 ||
                        iy == 0 || iy == Ny-1 ||
                        iz == 0 || iz == Nz-1);

    float f[Q];
#pragma unroll
    for (int q = 0; q < Q; ++q) {
        f[q] = f_src[q * N + idx];
    }

    float rho = 0.0f;
    float ux = 0.0f, uy = 0.0f, uz = 0.0f;
#pragma unroll
    for (int q = 0; q < Q; ++q) {
        rho += f[q];
        ux += f[q] * lbm_ex(q);
        uy += f[q] * lbm_ey(q);
        uz += f[q] * lbm_ez(q);
    }
    float inv_rho = 1.0f / fmaxf(rho, 1e-10f);
    ux *= inv_rho;
    uy *= inv_rho;
    uz *= inv_rho;

    float u2 = ux * ux + uy * uy + uz * uz;
    float f_post[Q];
#pragma unroll
    for (int q = 0; q < Q; ++q) {
        float eu = (float)lbm_ex(q) * ux + (float)lbm_ey(q) * uy + (float)lbm_ez(q) * uz;
        float f_eq = lbm_w(q) * rho * (1.0f + 3.0f * eu + 4.5f * eu * eu - 1.5f * u2);
        f_post[q] = f[q] + omega * (f_eq - f[q]);
    }

#pragma unroll
    for (int q = 0; q < Q; ++q) {
        int nx = ix + lbm_ex(q);
        int ny = iy + lbm_ey(q);
        int nz = iz + lbm_ez(q);

        if (is_boundary) {
            f_dst[lbm_opp(q) * N + idx] = f_post[q];
        } else if (nx >= 0 && nx < Nx && ny >= 0 && ny < Ny && nz >= 0 && nz < Nz) {
            int nidx = nz * Nx * Ny + ny * Nx + nx;
            f_dst[q * N + nidx] = f_post[q];
        } else {
            f_dst[lbm_opp(q) * N + idx] = f_post[q];
        }
    }
}
