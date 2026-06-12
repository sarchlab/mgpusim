// bicg.cu -- CUDA benchmark for PolyBench BiCG
// Two mat-vec products: q = A*p, s = A^T*r

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

__global__ void bicgKernel1(DATA_TYPE *A, DATA_TYPE *p, DATA_TYPE *q, int nx, int ny) {
    int i = blockDim.x * blockIdx.x + threadIdx.x;

    if (i < nx) {
        q[i] = 0.0f;
        for (int j = 0; j < ny; j++) {
            q[i] += A[i * ny + j] * p[j];
        }
    }
}

__global__ void bicgKernel2(DATA_TYPE *A, DATA_TYPE *r, DATA_TYPE *s, int nx, int ny) {
    int j = blockDim.x * blockIdx.x + threadIdx.x;

    if (j < ny) {
        s[j] = 0.0f;
        for (int i = 0; i < nx; i++) {
            s[j] += A[i * ny + j] * r[i];
        }
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int NX = parseIntParam(argc, argv, "--size", 256);
    int NY = NX;

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(NX * NY * sizeof(DATA_TYPE));
    DATA_TYPE* h_p = (DATA_TYPE*)malloc(NY * sizeof(DATA_TYPE));
    DATA_TYPE* h_r = (DATA_TYPE*)malloc(NX * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < NX * NY; i++)
        h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NY; i++)
        h_p[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NX; i++)
        h_r[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_p, *d_q, *d_r, *d_s;
    CUDA_CHECK(cudaMalloc(&d_A, NX * NY * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_p, NY * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_q, NX * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_r, NX * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_s, NY * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_A, h_A, NX * NY * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_p, h_p, NY * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_r, h_r, NX * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));

    const int THREADS = 256;
    dim3 block(THREADS);
    dim3 grid1((NX + THREADS - 1) / THREADS);
    dim3 grid2((NY + THREADS - 1) / THREADS);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", NX, NY);

    BenchResult r_bench = runBenchmark("bicg", problemSize, iterations, [&]() {
        bicgKernel1<<<grid1, block>>>(d_A, d_p, d_q, NX, NY);
        bicgKernel2<<<grid2, block>>>(d_A, d_r, d_s, NX, NY);
    });

    printCSVHeader();
    printCSVRow(r_bench);

    // Cleanup
    CUDA_CHECK(cudaFree(d_A));
    CUDA_CHECK(cudaFree(d_p));
    CUDA_CHECK(cudaFree(d_q));
    CUDA_CHECK(cudaFree(d_r));
    CUDA_CHECK(cudaFree(d_s));
    free(h_A);
    free(h_p);
    free(h_r);

    return 0;
}
