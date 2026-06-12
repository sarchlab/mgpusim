// hotspot.cu -- CUDA benchmark for Rodinia HotSpot
// 2D thermal simulation for chip temperature estimation (stencil).
// 1 kernel: iterative 2D stencil averaging neighbors.

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

#define AMB_TEMP 80.0f

// Chip parameters (simplified)
#define CHIP_HEIGHT 0.016f
#define CHIP_WIDTH 0.016f
#define T_CHIP 0.0005f
#define K_SI 100.0f
#define C_SI 1.75e6f
#define FACTOR_CHIP (T_CHIP / K_SI)

__global__ void hotspot_kernel(DATA_TYPE *temp_src, DATA_TYPE *temp_dst,
                               DATA_TYPE *power, int grid_cols, int grid_rows,
                               DATA_TYPE step_div_cap, DATA_TYPE Rx_1,
                               DATA_TYPE Ry_1, DATA_TYPE Rz_1) {
    int col = blockIdx.x * blockDim.x + threadIdx.x;
    int row = blockIdx.y * blockDim.y + threadIdx.y;

    if (col < grid_cols && row < grid_rows) {
        int idx = row * grid_cols + col;

        // Get neighbor temperatures (with boundary clamping)
        DATA_TYPE temp_c = temp_src[idx];
        DATA_TYPE temp_n =
            (row > 0) ? temp_src[(row - 1) * grid_cols + col] : temp_c;
        DATA_TYPE temp_s =
            (row < grid_rows - 1) ? temp_src[(row + 1) * grid_cols + col]
                                  : temp_c;
        DATA_TYPE temp_w =
            (col > 0) ? temp_src[row * grid_cols + (col - 1)] : temp_c;
        DATA_TYPE temp_e =
            (col < grid_cols - 1) ? temp_src[row * grid_cols + (col + 1)]
                                  : temp_c;

        // Compute the new temperature
        DATA_TYPE delta =
            step_div_cap *
            (power[idx] + (temp_n + temp_s - 2.0f * temp_c) * Ry_1 +
             (temp_w + temp_e - 2.0f * temp_c) * Rx_1 +
             (AMB_TEMP - temp_c) * Rz_1);

        temp_dst[idx] = temp_c + delta;
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int grid_size = parseIntParam(argc, argv, "--grid_size", 512);
    int block_dim = parseIntParam(argc, argv, "--block_size", 16);
    int num_iterations = parseIntParam(argc, argv, "--num_iterations", 10);

    int grid_rows = grid_size;
    int grid_cols = grid_size;
    int total_cells = grid_rows * grid_cols;

    // Compute thermal parameters
    DATA_TYPE grid_height = CHIP_HEIGHT / grid_rows;
    DATA_TYPE grid_width = CHIP_WIDTH / grid_cols;
    DATA_TYPE cap = C_SI * T_CHIP * grid_height * grid_width;
    DATA_TYPE Rx = grid_width / (2.0f * K_SI * T_CHIP * grid_height);
    DATA_TYPE Ry = grid_height / (2.0f * K_SI * T_CHIP * grid_width);
    DATA_TYPE Rz = T_CHIP / (K_SI * grid_height * grid_width);
    DATA_TYPE max_slope = K_SI / (0.5f * T_CHIP * C_SI);
    DATA_TYPE step = 0.001f / max_slope;
    DATA_TYPE step_div_cap = step / cap;
    DATA_TYPE Rx_1 = 1.0f / Rx;
    DATA_TYPE Ry_1 = 1.0f / Ry;
    DATA_TYPE Rz_1 = 1.0f / Rz;

    // Allocate host memory
    DATA_TYPE *h_temp = (DATA_TYPE *)malloc(total_cells * sizeof(DATA_TYPE));
    DATA_TYPE *h_power = (DATA_TYPE *)malloc(total_cells * sizeof(DATA_TYPE));
    DATA_TYPE *h_temp_orig =
        (DATA_TYPE *)malloc(total_cells * sizeof(DATA_TYPE));

    // Initialize data programmatically
    srand(42);
    for (int i = 0; i < total_cells; i++) {
        h_temp_orig[i] = AMB_TEMP + (DATA_TYPE)(rand() % 200) / 10.0f;
        h_power[i] = (DATA_TYPE)(rand() % 100) / 500.0f;
    }

    // Device allocations
    DATA_TYPE *d_temp_src, *d_temp_dst, *d_power;
    CUDA_CHECK(cudaMalloc(&d_temp_src, total_cells * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_temp_dst, total_cells * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_power, total_cells * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_power, h_power,
                          total_cells * sizeof(DATA_TYPE),
                          cudaMemcpyHostToDevice));

    dim3 block(block_dim, block_dim);
    dim3 grid((grid_cols + block_dim - 1) / block_dim,
              (grid_rows + block_dim - 1) / block_dim);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", grid_size);

    printCSVHeader();

    BenchResult r = runBenchmark(
        "hotspot", problemSize, iterations, [&]() {
            // Reset temperature for each benchmark iteration
            CUDA_CHECK(cudaMemcpy(d_temp_src, h_temp_orig,
                                  total_cells * sizeof(DATA_TYPE),
                                  cudaMemcpyHostToDevice));

            for (int iter = 0; iter < num_iterations; iter++) {
                hotspot_kernel<<<grid, block>>>(d_temp_src, d_temp_dst, d_power,
                                                grid_cols, grid_rows,
                                                step_div_cap, Rx_1, Ry_1,
                                                Rz_1);
                // Swap src and dst
                DATA_TYPE *tmp = d_temp_src;
                d_temp_src = d_temp_dst;
                d_temp_dst = tmp;
            }
            CUDA_CHECK(cudaDeviceSynchronize());
        });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_temp_src));
    CUDA_CHECK(cudaFree(d_temp_dst));
    CUDA_CHECK(cudaFree(d_power));
    free(h_temp);
    free(h_power);
    free(h_temp_orig);

    return 0;
}
