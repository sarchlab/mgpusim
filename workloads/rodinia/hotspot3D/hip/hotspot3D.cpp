// hotspot3D.cpp -- HIP benchmark for Rodinia HotSpot3D
// 3D thermal simulation (3D stencil kernel).
// 1 kernel: iterative 3D stencil where each cell averages 6 neighbors + self.

#include "bench_common_hip.h"

typedef float DATA_TYPE;

#define AMB_TEMP 80.0f
#define K_SI 100.0f
#define C_SI 1.75e6f
#define T_CHIP 0.0005f

__global__ void hotspot3D_kernel(DATA_TYPE *temp_src, DATA_TYPE *temp_dst,
                                 DATA_TYPE *power, int nx, int ny, int nz,
                                 DATA_TYPE ce, DATA_TYPE cw,
                                 DATA_TYPE cn, DATA_TYPE cs,
                                 DATA_TYPE ct, DATA_TYPE cb,
                                 DATA_TYPE cc, DATA_TYPE step_div_cap) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;  // x
    int j = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;  // y

    if (i < nx && j < ny) {
        for (int k = 0; k < nz; k++) {
            int idx = k * nx * ny + j * nx + i;

            DATA_TYPE temp_c = temp_src[idx];

            // Neighbors with boundary clamping
            DATA_TYPE temp_w = (i > 0)      ? temp_src[idx - 1]        : temp_c;
            DATA_TYPE temp_e = (i < nx - 1) ? temp_src[idx + 1]        : temp_c;
            DATA_TYPE temp_n = (j > 0)      ? temp_src[idx - nx]       : temp_c;
            DATA_TYPE temp_s = (j < ny - 1) ? temp_src[idx + nx]       : temp_c;
            DATA_TYPE temp_b = (k > 0)      ? temp_src[idx - nx * ny]  : temp_c;
            DATA_TYPE temp_t = (k < nz - 1) ? temp_src[idx + nx * ny]  : temp_c;

            temp_dst[idx] = temp_c * cc +
                            temp_w * cw + temp_e * ce +
                            temp_n * cn + temp_s * cs +
                            temp_b * cb + temp_t * ct +
                            step_div_cap * power[idx] +
                            ct * (AMB_TEMP - temp_c);
        }
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int grid_size = parseIntParam(argc, argv, "--grid_size", 64);
    int num_iterations = parseIntParam(argc, argv, "--num_iterations", 10);
    int block_x = parseIntParam(argc, argv, "--block_x", 8);
    int block_y = parseIntParam(argc, argv, "--block_y", 8);

    int nx = grid_size, ny = grid_size, nz = grid_size;
    long total_cells = (long)nx * ny * nz;

    // Compute thermal parameters
    DATA_TYPE dx = 0.016f / nx;
    DATA_TYPE dy = 0.016f / ny;
    DATA_TYPE dz = T_CHIP / nz;
    DATA_TYPE cap = C_SI * dz * dx * dy;
    DATA_TYPE Rx = dx / (2.0f * K_SI * dz * dy);
    DATA_TYPE Ry = dy / (2.0f * K_SI * dz * dx);
    DATA_TYPE Rz = dz / (K_SI * dx * dy);
    DATA_TYPE max_slope = K_SI / (0.5f * dz * C_SI);
    DATA_TYPE step = 0.001f / max_slope;
    DATA_TYPE step_div_cap = step / cap;

    DATA_TYPE ce = 1.0f / (2.0f * Rx) * step / cap;
    DATA_TYPE cw = 1.0f / (2.0f * Rx) * step / cap;
    DATA_TYPE cn = 1.0f / (2.0f * Ry) * step / cap;
    DATA_TYPE cs = 1.0f / (2.0f * Ry) * step / cap;
    DATA_TYPE ct = 1.0f / (2.0f * Rz) * step / cap;
    DATA_TYPE cb = 1.0f / (2.0f * Rz) * step / cap;
    DATA_TYPE cc = 1.0f - 2.0f * ce - 2.0f * cn - 2.0f * ct;

    // Allocate host memory
    DATA_TYPE *h_temp = (DATA_TYPE *)malloc(total_cells * sizeof(DATA_TYPE));
    DATA_TYPE *h_power = (DATA_TYPE *)malloc(total_cells * sizeof(DATA_TYPE));

    // Initialize data programmatically
    srand(42);
    for (long i = 0; i < total_cells; i++) {
        h_temp[i] = AMB_TEMP + (DATA_TYPE)(rand() % 200) / 10.0f;
        h_power[i] = (DATA_TYPE)(rand() % 100) / 500.0f;
    }

    // Device allocations
    DATA_TYPE *d_temp_src, *d_temp_dst, *d_power;
    HIP_CHECK(hipMalloc(&d_temp_src, total_cells * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_temp_dst, total_cells * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_power, total_cells * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_power, h_power, total_cells * sizeof(DATA_TYPE),
                        hipMemcpyHostToDevice));

    dim3 block(block_x, block_y);
    dim3 grid((nx + block_x - 1) / block_x, (ny + block_y - 1) / block_y);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", grid_size);

    printCSVHeader();

    BenchResult r = runBenchmark(
        "hotspot3D", problemSize, iterations, [&]() {
            // Reset temperature
            HIP_CHECK(hipMemcpy(d_temp_src, h_temp,
                                total_cells * sizeof(DATA_TYPE),
                                hipMemcpyHostToDevice));

            for (int iter = 0; iter < num_iterations; iter++) {
                hotspot3D_kernel<<<grid, block>>>(
                    d_temp_src, d_temp_dst, d_power,
                    nx, ny, nz, ce, cw, cn, cs, ct, cb, cc, step_div_cap);

                // Swap src and dst
                DATA_TYPE *tmp = d_temp_src;
                d_temp_src = d_temp_dst;
                d_temp_dst = tmp;
            }
            HIP_CHECK(hipDeviceSynchronize());
        });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_temp_src));
    HIP_CHECK(hipFree(d_temp_dst));
    HIP_CHECK(hipFree(d_power));
    free(h_temp);
    free(h_power);

    return 0;
}
