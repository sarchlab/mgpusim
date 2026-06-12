// gaussian.cu -- CUDA benchmark for Rodinia Gaussian Elimination
// Two kernels: Fan1 (pivot row division) and Fan2 (row elimination)
// Solves a randomly generated linear system Ax = b.

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

// Fan1: Divide pivot row elements by the pivot element
__global__ void Fan1(DATA_TYPE *M, DATA_TYPE *a, int size, int t) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx < size - 1 - t) {
        M[(size) * (t + 1 + idx) + t] =
            a[(size) * (t + 1 + idx) + t] / a[size * t + t];
    }
}

// Fan2: Eliminate elements below the pivot
__global__ void Fan2(DATA_TYPE *M, DATA_TYPE *a, DATA_TYPE *b, int size,
                     int t) {
    int xidx = blockIdx.x * blockDim.x + threadIdx.x;
    int yidx = blockIdx.y * blockDim.y + threadIdx.y;

    if (xidx < size - 1 - t && yidx < size - t) {
        int row = t + 1 + xidx;
        int col = t + 1 + yidx;
        a[size * row + col] -= M[size * row + t] * a[size * t + col];
        if (yidx == 0) {
            b[row] -= M[size * row + t] * b[t];
        }
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int size = parseIntParam(argc, argv, "--matrix_size", 256);
    int block_size = parseIntParam(argc, argv, "--block_size", 32);

    // Allocate host memory
    DATA_TYPE *h_a = (DATA_TYPE *)malloc(size * size * sizeof(DATA_TYPE));
    DATA_TYPE *h_b = (DATA_TYPE *)malloc(size * sizeof(DATA_TYPE));
    DATA_TYPE *h_m = (DATA_TYPE *)malloc(size * size * sizeof(DATA_TYPE));
    DATA_TYPE *h_a_orig = (DATA_TYPE *)malloc(size * size * sizeof(DATA_TYPE));
    DATA_TYPE *h_b_orig = (DATA_TYPE *)malloc(size * sizeof(DATA_TYPE));

    // Initialize data programmatically
    srand(42);
    for (int i = 0; i < size * size; i++)
        h_a_orig[i] = (DATA_TYPE)(rand() % 1000) / 100.0f;
    for (int i = 0; i < size; i++)
        h_b_orig[i] = (DATA_TYPE)(rand() % 1000) / 100.0f;
    // Make diagonally dominant to ensure solvability
    for (int i = 0; i < size; i++)
        h_a_orig[i * size + i] += (DATA_TYPE)size;

    // Device allocations
    DATA_TYPE *d_a, *d_b, *d_m;
    CUDA_CHECK(cudaMalloc(&d_a, size * size * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_b, size * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_m, size * size * sizeof(DATA_TYPE)));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", size);

    printCSVHeader();

    BenchResult r = runBenchmark("gaussian", problemSize, iterations, [&]() {
        // Reset data each iteration
        memcpy(h_a, h_a_orig, size * size * sizeof(DATA_TYPE));
        memcpy(h_b, h_b_orig, size * sizeof(DATA_TYPE));
        memset(h_m, 0, size * size * sizeof(DATA_TYPE));

        CUDA_CHECK(cudaMemcpy(d_a, h_a, size * size * sizeof(DATA_TYPE),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(d_b, h_b, size * sizeof(DATA_TYPE),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(d_m, h_m, size * size * sizeof(DATA_TYPE),
                              cudaMemcpyHostToDevice));

        for (int t = 0; t < size - 1; t++) {
            // Fan1: 1D grid
            int fan1_threads = size - 1 - t;
            if (fan1_threads > 0) {
                dim3 block1(block_size);
                dim3 grid1((fan1_threads + block_size - 1) / block_size);
                Fan1<<<grid1, block1>>>(d_m, d_a, size, t);
            }

            // Fan2: 2D grid
            int fan2_rows = size - 1 - t;
            int fan2_cols = size - t;
            if (fan2_rows > 0 && fan2_cols > 0) {
                dim3 block2(block_size, block_size);
                dim3 grid2((fan2_rows + block_size - 1) / block_size,
                           (fan2_cols + block_size - 1) / block_size);
                Fan2<<<grid2, block2>>>(d_m, d_a, d_b, size, t);
            }
            CUDA_CHECK(cudaDeviceSynchronize());
        }
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_a));
    CUDA_CHECK(cudaFree(d_b));
    CUDA_CHECK(cudaFree(d_m));
    free(h_a);
    free(h_b);
    free(h_m);
    free(h_a_orig);
    free(h_b_orig);

    return 0;
}
