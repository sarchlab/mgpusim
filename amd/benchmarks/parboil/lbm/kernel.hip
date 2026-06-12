#include <hip/hip_runtime.h>

// D3Q19 velocity directions
#define Q 19

// D3Q19 weights
__constant__ float d_w[Q] = {
    1.0f/3.0f,                                          // 0: rest
    1.0f/18.0f, 1.0f/18.0f, 1.0f/18.0f,                // 1-3: +x,+y,+z
    1.0f/18.0f, 1.0f/18.0f, 1.0f/18.0f,                // 4-6: -x,-y,-z
    1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f,    // 7-10: xy diags
    1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f,    // 11-14: xz diags
    1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f     // 15-18: yz diags
};

// D3Q19 velocity vectors (ex, ey, ez)
__constant__ int d_ex[Q] = { 0,  1, 0, 0, -1,  0,  0,  1,-1, 1,-1,  1,-1, 1,-1,  0, 0, 0, 0};
__constant__ int d_ey[Q] = { 0,  0, 1, 0,  0, -1,  0,  1, 1,-1,-1,  0, 0, 0, 0,  1,-1, 1,-1};
__constant__ int d_ez[Q] = { 0,  0, 0, 1,  0,  0, -1,  0, 0, 0, 0,  1, 1,-1,-1,  1, 1,-1,-1};

// Opposite direction indices for bounce-back
__constant__ int d_opp[Q] = { 0,  4, 5, 6,  1,  2,  3, 10, 9, 8, 7, 14,13,12,11, 18,17,16,15};

// Stream + Collide kernel
// src: source distribution, dst: destination distribution
// N: grid dimension (NxNxN), omega: relaxation parameter
__global__ void lbm_stream_collide(float *src, float *dst, int N, float omega) {
    int idx = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int total = N * N * N;
    if (idx >= total) return;

    // Convert linear index to 3D
    int z = idx / (N * N);
    int y = (idx / N) % N;
    int x = idx % N;

    // Streaming: gather from neighbors
    float f[Q];
    for (int q = 0; q < Q; q++) {
        // Source cell for direction q (streaming step: pull from neighbor)
        int sx = (x - d_ex[q] + N) % N;
        int sy = (y - d_ey[q] + N) % N;
        int sz = (z - d_ez[q] + N) % N;
        int src_idx = (sz * N * N + sy * N + sx) * Q + q;
        f[q] = src[src_idx];
    }

    // Compute macroscopic quantities: density and velocity
    float rho = 0.0f;
    float ux = 0.0f, uy = 0.0f, uz = 0.0f;
    for (int q = 0; q < Q; q++) {
        rho += f[q];
        ux += f[q] * d_ex[q];
        uy += f[q] * d_ey[q];
        uz += f[q] * d_ez[q];
    }

    // Avoid division by zero
    float inv_rho = (rho > 1e-10f) ? (1.0f / rho) : 0.0f;
    ux *= inv_rho;
    uy *= inv_rho;
    uz *= inv_rho;

    // Collision step: BGK relaxation toward equilibrium
    float u_sq = ux * ux + uy * uy + uz * uz;
    for (int q = 0; q < Q; q++) {
        float eu = (float)d_ex[q] * ux + (float)d_ey[q] * uy + (float)d_ez[q] * uz;
        float feq = d_w[q] * rho * (1.0f + 3.0f * eu + 4.5f * eu * eu - 1.5f * u_sq);
        f[q] = f[q] * (1.0f - omega) + feq * omega;
    }

    // Write results to destination
    for (int q = 0; q < Q; q++) {
        dst[idx * Q + q] = f[q];
    }
}
