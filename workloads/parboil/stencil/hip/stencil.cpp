// stencil.cpp -- HIP benchmark for Parboil 3D Jacobi Stencil
// 7-point 3D stencil (center + 6 face neighbors). Ping-pong between two arrays
// across timesteps. Boundary cells stay fixed.

#include "bench_common_hip.h"

typedef float DATA_TYPE;

// 7-point 3D stencil kernel
__global__ void stencil_kernel(DATA_TYPE *in, DATA_TYPE *out,
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
                   coeff_neighbor * (in[idx - 1] +       // x-1
                                     in[idx + 1] +       // x+1
                                     in[idx - nx] +      // y-1
                                     in[idx + nx] +      // y+1
                                     in[idx - ny * nx] +  // z-1
                                     in[idx + ny * nx]);   // z+1
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int grid_dim = parseIntParam(argc, argv, "--grid_dim", 64);
    int num_timesteps = parseIntParam(argc, argv, "--num_timesteps", 10);
    int coeff_center_x100 = parseIntParam(argc, argv, "--coeff_center_x100", 60);
    int block_dim_x = parseIntParam(argc, argv, "--block_dim_x", 16);
    int block_dim_y = parseIntParam(argc, argv, "--block_dim_y", 16);

    DATA_TYPE coeff_center = coeff_center_x100 / 100.0f;
    DATA_TYPE coeff_neighbor = (1.0f - coeff_center) / 6.0f;

    int nx = grid_dim;
    int ny = grid_dim;
    int nz = grid_dim;
    int total = nx * ny * nz;

    // Allocate host memory
    DATA_TYPE *h_data = (DATA_TYPE *)malloc(total * sizeof(DATA_TYPE));

    // Initialize data programmatically
    srand(42);
    for (int i = 0; i < total; i++) {
        h_data[i] = (DATA_TYPE)(rand() % 1000) / 1000.0f;
    }

    // Device allocations (two arrays for ping-pong)
    DATA_TYPE *d_A, *d_B;
    HIP_CHECK(hipMalloc(&d_A, total * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_B, total * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_A, h_data, total * sizeof(DATA_TYPE),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_B, h_data, total * sizeof(DATA_TYPE),
                        hipMemcpyHostToDevice));

    // Kernel launch config
    dim3 block(block_dim_x, block_dim_y, 1);
    dim3 grid((nx + block_dim_x - 1) / block_dim_x,
              (ny + block_dim_y - 1) / block_dim_y,
              nz);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", grid_dim);

    printCSVHeader();

    // Benchmark stencil kernel (multiple timesteps per iteration)
    BenchResult r1 = runBenchmark(
        "stencil_kernel", problemSize, iterations, [&]() {
            for (int t = 0; t < num_timesteps; t++) {
                if (t % 2 == 0) {
                    stencil_kernel<<<grid, block>>>(d_A, d_B, nx, ny, nz,
                                                    coeff_center, coeff_neighbor);
                } else {
                    stencil_kernel<<<grid, block>>>(d_B, d_A, nx, ny, nz,
                                                    coeff_center, coeff_neighbor);
                }
            }
        });
    printCSVRow(r1);

    // Cleanup
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_B));
    free(h_data);

    return 0;
}
