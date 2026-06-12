// correlation.cpp -- HIP benchmark for PolyBench Correlation
// Four kernels: mean, stddev, normalize, correlation matrix
// Input: N×M data matrix
// Output: M×M correlation matrix

#include "bench_common_hip.h"

typedef float DATA_TYPE;

#define FLOAT_N 3214212.01f
#define EPS     0.005f

__global__ void mean_kernel(DATA_TYPE *data, DATA_TYPE *mean, int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (j < M) {
        DATA_TYPE sum = 0.0f;
        for (int i = 0; i < N; i++) {
            sum += data[i * M + j];
        }
        mean[j] = sum / (DATA_TYPE)N;
    }
}

__global__ void stddev_kernel(DATA_TYPE *data, DATA_TYPE *mean, DATA_TYPE *stddev,
                              int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (j < M) {
        DATA_TYPE sum = 0.0f;
        for (int i = 0; i < N; i++) {
            DATA_TYPE diff = data[i * M + j] - mean[j];
            sum += diff * diff;
        }
        stddev[j] = sqrtf(sum / (DATA_TYPE)N);
        if (stddev[j] < EPS) stddev[j] = 1.0f;
    }
}

__global__ void normalize_kernel(DATA_TYPE *data, DATA_TYPE *mean, DATA_TYPE *stddev,
                                 int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < N && j < M) {
        data[i * M + j] = (data[i * M + j] - mean[j]) / (sqrtf((DATA_TYPE)N) * stddev[j]);
    }
}

__global__ void corr_kernel(DATA_TYPE *data, DATA_TYPE *corr, int N, int M) {
    int j1 = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < M && j1 < M) {
        if (i == j1) {
            corr[i * M + j1] = 1.0f;
        } else if (i < j1) {
            DATA_TYPE sum = 0.0f;
            for (int k = 0; k < N; k++) {
                sum += data[k * M + i] * data[k * M + j1];
            }
            corr[i * M + j1] = sum;
            corr[j1 * M + i] = sum;
        }
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    int M = N;

    size_t dataSize = (size_t)N * M * sizeof(DATA_TYPE);
    size_t corrSize = (size_t)M * M * sizeof(DATA_TYPE);

    // Host allocations
    DATA_TYPE* h_data = (DATA_TYPE*)malloc(dataSize);
    srand(42);
    for (int i = 0; i < N * M; i++)
        h_data[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_data, *d_mean, *d_stddev, *d_corr;
    HIP_CHECK(hipMalloc(&d_data, dataSize));
    HIP_CHECK(hipMalloc(&d_mean, M * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_stddev, M * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_corr, corrSize));

    const int THREADS = 256;
    dim3 block1D(THREADS);
    dim3 gridM((M + THREADS - 1) / THREADS);

    dim3 block2D(16, 16);
    dim3 gridNorm((M + 15) / 16, (N + 15) / 16);
    dim3 gridCorr((M + 15) / 16, (M + 15) / 16);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", N, M);

    printCSVHeader();
    BenchResult r = runBenchmark("correlation", problemSize, iterations, [&]() {
        // Re-upload data each iteration (normalize modifies it)
        HIP_CHECK(hipMemcpy(d_data, h_data, dataSize, hipMemcpyHostToDevice));
        mean_kernel<<<gridM, block1D>>>(d_data, d_mean, N, M);
        stddev_kernel<<<gridM, block1D>>>(d_data, d_mean, d_stddev, N, M);
        normalize_kernel<<<gridNorm, block2D>>>(d_data, d_mean, d_stddev, N, M);
        corr_kernel<<<gridCorr, block2D>>>(d_data, d_corr, N, M);
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_data));
    HIP_CHECK(hipFree(d_mean));
    HIP_CHECK(hipFree(d_stddev));
    HIP_CHECK(hipFree(d_corr));
    free(h_data);

    return 0;
}
