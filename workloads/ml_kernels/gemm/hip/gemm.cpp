// gemm.cpp -- HIP tiled shared-memory GEMM benchmark
// Computes C = alpha * A * B + beta * C with configurable tile sizes.
// Uses shared memory tiling for efficient matrix multiplication.
// Reports GFLOPS = (2 * M * N * K) / (time_sec * 1e9).

#include "bench_common_hip.h"

// Maximum tile size (compile-time). Runtime tile_size must be <= this.
#define MAX_TILE_SIZE 32

// ---------------------------------------------------------------------------
// Tiled GEMM kernel -- tile_size selected at compile time via template
// ---------------------------------------------------------------------------
template <int TILE_SIZE>
__global__ void tiled_gemm(const float* __restrict__ A,
                           const float* __restrict__ B,
                           float*       __restrict__ C,
                           int M, int N, int K,
                           float alpha, float beta) {
    __shared__ float As[TILE_SIZE][TILE_SIZE];
    __shared__ float Bs[TILE_SIZE][TILE_SIZE];

    int row = hipBlockIdx_y * TILE_SIZE + hipThreadIdx_y;
    int col = hipBlockIdx_x * TILE_SIZE + hipThreadIdx_x;
    float acc = 0.0f;

    int numTiles = (K + TILE_SIZE - 1) / TILE_SIZE;

    for (int t = 0; t < numTiles; t++) {
        int aCol = t * TILE_SIZE + hipThreadIdx_x;
        int bRow = t * TILE_SIZE + hipThreadIdx_y;

        As[hipThreadIdx_y][hipThreadIdx_x] =
            (row < M && aCol < K) ? A[row * K + aCol] : 0.0f;
        Bs[hipThreadIdx_y][hipThreadIdx_x] =
            (bRow < K && col < N) ? B[bRow * N + col] : 0.0f;

        __syncthreads();

        #pragma unroll
        for (int k = 0; k < TILE_SIZE; k++) {
            acc += As[hipThreadIdx_y][k] * Bs[k][hipThreadIdx_x];
        }

        __syncthreads();
    }

    if (row < M && col < N) {
        C[row * N + col] = alpha * acc + beta * C[row * N + col];
    }
}

// ---------------------------------------------------------------------------
// Decode NK_shape preset to actual N, K values
// ---------------------------------------------------------------------------
static void decodeNKShape(int shape, int &N, int &K) {
    switch (shape) {
        case 1: N = 768;   K = 768;  break; // GPT-2 attention output projection
        case 2: N = 2304;  K = 768;  break; // GPT-2 QKV combined projection
        case 3: N = 3072;  K = 768;  break; // GPT-2 FFN up-projection
        case 4: N = 4096;  K = 4096; break; // Llama-2-7B attention
        case 5: N = 11008; K = 4096; break; // Llama-2-7B FFN gate/up
        default:
            fprintf(stderr, "Unknown NK_shape %d. Use 1-5.\n", shape);
            exit(1);
    }
}

// ---------------------------------------------------------------------------
// Parse a float parameter from command line
// ---------------------------------------------------------------------------
static float parseFloatParam(int argc, char** argv, const char* name,
                             float defaultVal) {
    for (int i = 1; i < argc - 1; ++i) {
        if (strcmp(argv[i], name) == 0) {
            float v = (float)atof(argv[i + 1]);
            return v;
        }
    }
    return defaultVal;
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int M          = parseIntParam(argc, argv, "--M", 1024);
    int nk_shape   = parseIntParam(argc, argv, "--NK_shape", 1);
    int tile_size  = parseIntParam(argc, argv, "--tile_size", 16);
    float alpha    = parseFloatParam(argc, argv, "--alpha", 1.0f);
    float beta     = 0.0f; // beta fixed at 0 for pure GEMM

    int N, K;
    decodeNKShape(nk_shape, N, K);

    // Allocate host arrays and initialize with deterministic random data
    size_t sizeA = (size_t)M * K * sizeof(float);
    size_t sizeB = (size_t)K * N * sizeof(float);
    size_t sizeC = (size_t)M * N * sizeof(float);

    float* h_A = (float*)malloc(sizeA);
    float* h_B = (float*)malloc(sizeB);
    float* h_C = (float*)malloc(sizeC);

    srand(42);
    for (size_t i = 0; i < (size_t)M * K; i++)
        h_A[i] = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
    for (size_t i = 0; i < (size_t)K * N; i++)
        h_B[i] = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
    for (size_t i = 0; i < (size_t)M * N; i++)
        h_C[i] = 0.0f;

    // Allocate device memory
    float *d_A, *d_B, *d_C;
    HIP_CHECK(hipMalloc(&d_A, sizeA));
    HIP_CHECK(hipMalloc(&d_B, sizeB));
    HIP_CHECK(hipMalloc(&d_C, sizeC));

    HIP_CHECK(hipMemcpy(d_A, h_A, sizeA, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_B, h_B, sizeB, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_C, h_C, sizeC, hipMemcpyHostToDevice));

    // Problem size string
    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "M%d_N%d_K%d_tile%d",
             M, N, K, tile_size);

    printCSVHeader();

    // Compute FLOPS: 2*M*N*K (multiply-add per output element)
    double total_flops = 2.0 * (double)M * (double)N * (double)K;

    BenchResult r;

    if (tile_size == 8) {
        dim3 block(8, 8);
        dim3 grid((N + 7) / 8, (M + 7) / 8);

        r = runBenchmark("tiled_gemm_8", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(tiled_gemm<8>, grid, block, 0, 0,
                               d_A, d_B, d_C, M, N, K, alpha, beta);
        });
    } else if (tile_size == 16) {
        dim3 block(16, 16);
        dim3 grid((N + 15) / 16, (M + 15) / 16);

        r = runBenchmark("tiled_gemm_16", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(tiled_gemm<16>, grid, block, 0, 0,
                               d_A, d_B, d_C, M, N, K, alpha, beta);
        });
    } else {
        // Default to tile_size 32
        dim3 block(32, 32);
        dim3 grid((N + 31) / 32, (M + 31) / 32);

        r = runBenchmark("tiled_gemm_32", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(tiled_gemm<32>, grid, block, 0, 0,
                               d_A, d_B, d_C, M, N, K, alpha, beta);
        });
    }

    printCSVRow(r);

    double gflops = (total_flops / 1.0e9) / (r.avg_ms / 1000.0);
    printf("# M=%d, N=%d, K=%d, tile_size=%d\n", M, N, K, tile_size);
    printf("# Total FLOPs: %.2e\n", total_flops);
    printf("# Throughput: %.2f GFLOPS\n", gflops);

    // Cleanup
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_B));
    HIP_CHECK(hipFree(d_C));
    free(h_A);
    free(h_B);
    free(h_C);

    return 0;
}
