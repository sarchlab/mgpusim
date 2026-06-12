// atax.cu -- CUDA benchmark for PolyBench ATAX (A^T * (A * x))
// Two kernels: atax_kernel1 computes tmp=A*x, atax_kernel2 computes y=A^T*tmp

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

__global__ void atax_kernel1(DATA_TYPE *A, DATA_TYPE *x, DATA_TYPE *tmp, int nx, int ny) {
    int i = blockDim.x * blockIdx.x + threadIdx.x;

    if (i < nx) {
        DATA_TYPE sum = 0.0f;
        for (int j = 0; j < ny; j++) {
            sum += A[i * ny + j] * x[j];
        }
        tmp[i] += sum;
    }
}

__global__ void atax_kernel2(DATA_TYPE *A, DATA_TYPE *y, DATA_TYPE *tmp, int nx, int ny) {
    int j = blockDim.x * blockIdx.x + threadIdx.x;

    if (j < ny) {
        DATA_TYPE sum = 0.0f;
        for (int i = 0; i < nx; i++) {
            sum += A[i * ny + j] * tmp[i];
        }
        y[j] += sum;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int NX = parseIntParam(argc, argv, "--size", 256);
    int NY = NX;

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(NX * NY * sizeof(DATA_TYPE));
    DATA_TYPE* h_x = (DATA_TYPE*)malloc(NY * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < NX * NY; i++)
        h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NY; i++)
        h_x[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_x, *d_tmp, *d_y;
    CUDA_CHECK(cudaMalloc(&d_A, NX * NY * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_x, NY * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_tmp, NX * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_y, NY * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_A, h_A, NX * NY * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_x, h_x, NY * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));

    const int THREADS = 256;
    dim3 block(THREADS);
    dim3 grid1((NX + THREADS - 1) / THREADS);
    dim3 grid2((NY + THREADS - 1) / THREADS);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", NX, NY);

    BenchResult r = runBenchmark("atax", problemSize, iterations, [&]() {
        // Reset tmp and y each iteration
        CUDA_CHECK(cudaMemset(d_tmp, 0, NX * sizeof(DATA_TYPE)));
        CUDA_CHECK(cudaMemset(d_y, 0, NY * sizeof(DATA_TYPE)));
        atax_kernel1<<<grid1, block>>>(d_A, d_x, d_tmp, NX, NY);
        atax_kernel2<<<grid2, block>>>(d_A, d_y, d_tmp, NX, NY);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_A));
    CUDA_CHECK(cudaFree(d_x));
    CUDA_CHECK(cudaFree(d_tmp));
    CUDA_CHECK(cudaFree(d_y));
    free(h_A);
    free(h_x);

    return 0;
}
