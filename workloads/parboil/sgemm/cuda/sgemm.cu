// sgemm.cu -- CUDA benchmark for Parboil SGEMM (Single-precision General Matrix Multiply)
// Tiled SGEMM using shared memory: C = alpha*A*B + beta*C

#include "bench_common_cuda.h"

// Tiled SGEMM kernel using shared memory
// TILE_WIDTH is passed as a template parameter
template <int TILE_WIDTH>
__global__ void sgemm_tiled(const float *A, const float *B, float *C,
                            int N, float alpha, float beta) {
    __shared__ float tileA[TILE_WIDTH][TILE_WIDTH];
    __shared__ float tileB[TILE_WIDTH][TILE_WIDTH];

    int row = blockIdx.y * TILE_WIDTH + threadIdx.y;
    int col = blockIdx.x * TILE_WIDTH + threadIdx.x;

    float sum = 0.0f;
    int numTiles = (N + TILE_WIDTH - 1) / TILE_WIDTH;

    for (int t = 0; t < numTiles; t++) {
        // Load tile of A
        int aCol = t * TILE_WIDTH + threadIdx.x;
        if (row < N && aCol < N)
            tileA[threadIdx.y][threadIdx.x] = A[row * N + aCol];
        else
            tileA[threadIdx.y][threadIdx.x] = 0.0f;

        // Load tile of B
        int bRow = t * TILE_WIDTH + threadIdx.y;
        if (bRow < N && col < N)
            tileB[threadIdx.y][threadIdx.x] = B[bRow * N + col];
        else
            tileB[threadIdx.y][threadIdx.x] = 0.0f;

        __syncthreads();

        // Compute partial dot product for this tile
        for (int k = 0; k < TILE_WIDTH; k++) {
            sum += tileA[threadIdx.y][k] * tileB[k][threadIdx.x];
        }

        __syncthreads();
    }

    // Write result: C = alpha * A*B + beta * C
    if (row < N && col < N) {
        int idx = row * N + col;
        C[idx] = alpha * sum + beta * C[idx];
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--matrix_size", 512);
    int tile_size = parseIntParam(argc, argv, "--tile_size", 16);
    float alpha = parseIntParam(argc, argv, "--alpha_x10", 10) / 10.0f;
    float beta = parseIntParam(argc, argv, "--beta_x10", 0) / 10.0f;
    int block_dim = parseIntParam(argc, argv, "--block_dim", 16);

    // Use tile_size as the effective block dimension for the tiled kernel
    // block_dim parameter is ignored if tile_size is set (tile_size drives shared memory tile)
    (void)block_dim;

    size_t matSize = (size_t)N * N * sizeof(float);

    // Allocate host memory
    float *h_A = (float *)malloc(matSize);
    float *h_B = (float *)malloc(matSize);
    float *h_C = (float *)malloc(matSize);

    // Initialize data programmatically
    srand(42);
    for (int i = 0; i < N * N; i++) {
        h_A[i] = (float)(rand() % 100) / 100.0f;
        h_B[i] = (float)(rand() % 100) / 100.0f;
        h_C[i] = (float)(rand() % 100) / 100.0f;
    }

    // Device allocations
    float *d_A, *d_B, *d_C;
    CUDA_CHECK(cudaMalloc(&d_A, matSize));
    CUDA_CHECK(cudaMalloc(&d_B, matSize));
    CUDA_CHECK(cudaMalloc(&d_C, matSize));

    CUDA_CHECK(cudaMemcpy(d_A, h_A, matSize, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_B, h_B, matSize, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_C, h_C, matSize, cudaMemcpyHostToDevice));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();

    // Launch kernel based on tile_size
    if (tile_size == 8) {
        dim3 block(8, 8);
        dim3 grid((N + 7) / 8, (N + 7) / 8);
        BenchResult r = runBenchmark(
            "sgemm_tiled", problemSize, iterations, [&]() {
                sgemm_tiled<8><<<grid, block>>>(d_A, d_B, d_C, N, alpha, beta);
            });
        printCSVRow(r);
    } else if (tile_size == 32) {
        dim3 block(32, 32);
        dim3 grid((N + 31) / 32, (N + 31) / 32);
        BenchResult r = runBenchmark(
            "sgemm_tiled", problemSize, iterations, [&]() {
                sgemm_tiled<32><<<grid, block>>>(d_A, d_B, d_C, N, alpha, beta);
            });
        printCSVRow(r);
    } else {
        // Default tile_size = 16
        dim3 block(16, 16);
        dim3 grid((N + 15) / 16, (N + 15) / 16);
        BenchResult r = runBenchmark(
            "sgemm_tiled", problemSize, iterations, [&]() {
                sgemm_tiled<16><<<grid, block>>>(d_A, d_B, d_C, N, alpha, beta);
            });
        printCSVRow(r);
    }

    // Cleanup
    CUDA_CHECK(cudaFree(d_A));
    CUDA_CHECK(cudaFree(d_B));
    CUDA_CHECK(cudaFree(d_C));
    free(h_A);
    free(h_B);
    free(h_C);

    return 0;
}
