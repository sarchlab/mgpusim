// covariance.cu -- CUDA benchmark for PolyBench Covariance
// Three kernels: mean, reduce (subtract mean), covariance matrix
// Input: N×M data matrix
// Output: M×M covariance matrix

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

__global__ void mean_kernel(DATA_TYPE *data, DATA_TYPE *mean, int N, int M) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;

    if (j < M) {
        DATA_TYPE sum = 0.0f;
        for (int i = 0; i < N; i++) {
            sum += data[i * M + j];
        }
        mean[j] = sum / (DATA_TYPE)N;
    }
}

__global__ void reduce_kernel(DATA_TYPE *data, DATA_TYPE *mean, int N, int M) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < N && j < M) {
        data[i * M + j] -= mean[j];
    }
}

__global__ void covar_kernel(DATA_TYPE *data, DATA_TYPE *cov, int N, int M) {
    int j1 = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < M && j1 < M && i <= j1) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < N; k++) {
            sum += data[k * M + i] * data[k * M + j1];
        }
        DATA_TYPE val = sum / (DATA_TYPE)(N - 1);
        cov[i * M + j1] = val;
        cov[j1 * M + i] = val;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    int M = N;

    size_t dataSize = (size_t)N * M * sizeof(DATA_TYPE);
    size_t covSize = (size_t)M * M * sizeof(DATA_TYPE);

    // Host allocations
    DATA_TYPE* h_data = (DATA_TYPE*)malloc(dataSize);
    srand(42);
    for (int i = 0; i < N * M; i++)
        h_data[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_data, *d_mean, *d_cov;
    CUDA_CHECK(cudaMalloc(&d_data, dataSize));
    CUDA_CHECK(cudaMalloc(&d_mean, M * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_cov, covSize));

    const int THREADS = 256;
    dim3 block1D(THREADS);
    dim3 gridM((M + THREADS - 1) / THREADS);

    dim3 block2D(16, 16);
    dim3 gridReduce((M + 15) / 16, (N + 15) / 16);
    dim3 gridCovar((M + 15) / 16, (M + 15) / 16);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", N, M);

    printCSVHeader();
    BenchResult r = runBenchmark("covariance", problemSize, iterations, [&]() {
        // Re-upload data each iteration (reduce modifies it)
        CUDA_CHECK(cudaMemcpy(d_data, h_data, dataSize, cudaMemcpyHostToDevice));
        mean_kernel<<<gridM, block1D>>>(d_data, d_mean, N, M);
        reduce_kernel<<<gridReduce, block2D>>>(d_data, d_mean, N, M);
        covar_kernel<<<gridCovar, block2D>>>(d_data, d_cov, N, M);
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_data));
    CUDA_CHECK(cudaFree(d_mean));
    CUDA_CHECK(cudaFree(d_cov));
    free(h_data);

    return 0;
}
