// lbm.cpp -- HIP benchmark for Parboil Lattice-Boltzmann Method (D3Q19)
// Kernel: Each thread updates one lattice cell by streaming + collision
// using D3Q19 model with two arrays (ping-pong) for src/dst.

#include "bench_common_hip.h"

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

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int grid_dim = parseIntParam(argc, argv, "--grid_dim", 32);
    int num_timesteps = parseIntParam(argc, argv, "--num_timesteps", 10);
    int omega_x100 = parseIntParam(argc, argv, "--omega_x100", 160);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);

    float omega = omega_x100 / 100.0f;
    int total_cells = grid_dim * grid_dim * grid_dim;
    size_t lattice_size = (size_t)total_cells * Q * sizeof(float);

    // Allocate host memory
    float *h_lattice = (float *)malloc(lattice_size);

    // Initialize with equilibrium distribution (rho=1, u=0)
    srand(42);
    for (int i = 0; i < total_cells; i++) {
        // Small random perturbation around equilibrium
        float rho = 1.0f + 0.01f * ((rand() % 200) - 100) / 100.0f;
        // D3Q19 weights for equilibrium at rest
        float w[Q] = {
            1.0f/3.0f,
            1.0f/18.0f, 1.0f/18.0f, 1.0f/18.0f,
            1.0f/18.0f, 1.0f/18.0f, 1.0f/18.0f,
            1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f,
            1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f,
            1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f, 1.0f/36.0f
        };
        for (int q = 0; q < Q; q++) {
            h_lattice[i * Q + q] = rho * w[q];
        }
    }

    // Device allocations (two arrays for ping-pong)
    float *d_src, *d_dst;
    HIP_CHECK(hipMalloc(&d_src, lattice_size));
    HIP_CHECK(hipMalloc(&d_dst, lattice_size));

    HIP_CHECK(hipMemcpy(d_src, h_lattice, lattice_size,
                        hipMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((total_cells + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", grid_dim);

    printCSVHeader();

    // Benchmark: run num_timesteps of stream+collide per iteration
    BenchResult r = runBenchmark(
        "lbm_stream_collide", problemSize, iterations, [&]() {
            for (int t = 0; t < num_timesteps; t++) {
                lbm_stream_collide<<<grid, block>>>(d_src, d_dst, grid_dim, omega);
                // Swap src and dst (ping-pong)
                float *tmp = d_src;
                d_src = d_dst;
                d_dst = tmp;
            }
        });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_src));
    HIP_CHECK(hipFree(d_dst));
    free(h_lattice);

    return 0;
}
