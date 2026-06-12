// pathfinder.cu -- CUDA benchmark for Rodinia Pathfinder
// Dynamic programming shortest path on a 2D grid (iterative stencil).
// 1 kernel launched per row: each row depends on previous row
// (min of 3 neighbors + cell cost).

#include "bench_common_cuda.h"

typedef int DATA_TYPE;

__global__ void pathfinder_kernel(DATA_TYPE *d_src, DATA_TYPE *d_dst,
                                  DATA_TYPE *d_wall, int width, int row) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;

    if (tid < width) {
        int offset = row * width;
        DATA_TYPE prev = d_src[tid];
        DATA_TYPE left = (tid > 0) ? d_src[tid - 1] : prev;
        DATA_TYPE right = (tid < width - 1) ? d_src[tid + 1] : prev;

        // Minimum of three neighbors from previous row
        DATA_TYPE min_val = prev;
        if (left < min_val) min_val = left;
        if (right < min_val) min_val = right;

        d_dst[tid] = d_wall[offset + tid] + min_val;
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int width = parseIntParam(argc, argv, "--width", 65536);
    int num_rows = parseIntParam(argc, argv, "--num_rows", 100);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);

    long total_cells = (long)width * num_rows;

    // Allocate host memory
    DATA_TYPE *h_wall = (DATA_TYPE *)malloc(total_cells * sizeof(DATA_TYPE));
    DATA_TYPE *h_result = (DATA_TYPE *)malloc(width * sizeof(DATA_TYPE));

    // Generate random cost matrix
    srand(42);
    for (long i = 0; i < total_cells; i++) {
        h_wall[i] = rand() % 10;
    }

    // Initialize first row as the starting costs
    for (int j = 0; j < width; j++) {
        h_result[j] = h_wall[j];
    }

    // Device allocations
    DATA_TYPE *d_wall, *d_src, *d_dst;
    CUDA_CHECK(cudaMalloc(&d_wall, total_cells * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_src, width * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_dst, width * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_wall, h_wall, total_cells * sizeof(DATA_TYPE),
                          cudaMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((width + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", width);

    printCSVHeader();

    BenchResult r = runBenchmark(
        "pathfinder", problemSize, iterations, [&]() {
            // Reset first row
            CUDA_CHECK(cudaMemcpy(d_src, h_result, width * sizeof(DATA_TYPE),
                                  cudaMemcpyHostToDevice));

            // Process each row iteratively
            for (int row = 1; row < num_rows; row++) {
                pathfinder_kernel<<<grid, block>>>(d_src, d_dst, d_wall,
                                                   width, row);

                // Swap src and dst
                DATA_TYPE *tmp = d_src;
                d_src = d_dst;
                d_dst = tmp;
            }
            CUDA_CHECK(cudaDeviceSynchronize());
        });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_wall));
    CUDA_CHECK(cudaFree(d_src));
    CUDA_CHECK(cudaFree(d_dst));
    free(h_wall);
    free(h_result);

    return 0;
}
