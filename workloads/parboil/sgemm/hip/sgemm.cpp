// sgemm.cpp -- HIP benchmark for Parboil SGEMM (Single-precision General Matrix Multiply)
// Tiled SGEMM using shared memory: C = alpha*A*B + beta*C

#include "bench_common_hip.h"

// Tiled SGEMM kernel using shared memory
template <int TILE_WIDTH>
__global__ void sgemm_tiled(const float *A, const float *B, float *C,
                            int N, float alpha, float beta) {
    __shared__ float tileA[TILE_WIDTH][TILE_WIDTH];
    __shared__ float tileB[TILE_WIDTH][TILE_WIDTH];

    int row = hipBlockIdx_y * TILE_WIDTH + hipThreadIdx_y;
    int col = hipBlockIdx_x * TILE_WIDTH + hipThreadIdx_x;

    float sum = 0.0f;
    int numTiles = (N + TILE_WIDTH - 1) / TILE_WIDTH;

    for (int t = 0; t < numTiles; t++) {
        // Load tile of A
        int aCol = t * TILE_WIDTH + hipThreadIdx_x;
        if (row < N && aCol < N)
            tileA[hipThreadIdx_y][hipThreadIdx_x] = A[row * N + aCol];
        else
            tileA[hipThreadIdx_y][hipThreadIdx_x] = 0.0f;

        // Load tile of B
        int bRow = t * TILE_WIDTH + hipThreadIdx_y;
        if (bRow < N && col < N)
            tileB[hipThreadIdx_y][hipThreadIdx_x] = B[bRow * N + col];
        else
            tileB[hipThreadIdx_y][hipThreadIdx_x] = 0.0f;

        __syncthreads();

        // Compute partial dot product for this tile
        for (int k = 0; k < TILE_WIDTH; k++) {
            sum += tileA[hipThreadIdx_y][k] * tileB[k][hipThreadIdx_x];
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
    HIP_CHECK(hipMalloc(&d_A, matSize));
    HIP_CHECK(hipMalloc(&d_B, matSize));
    HIP_CHECK(hipMalloc(&d_C, matSize));

    HIP_CHECK(hipMemcpy(d_A, h_A, matSize, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_B, h_B, matSize, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_C, h_C, matSize, hipMemcpyHostToDevice));

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
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_B));
    HIP_CHECK(hipFree(d_C));
    free(h_A);
    free(h_B);
    free(h_C);

    return 0;
}
