// atax.cpp — HIP benchmark for PolyBench ATAX (A'*A*x)
// Kernels copied from amd/benchmarks/polybench/atax/native/atax.cpp
// Problem size: nx=256, ny=256

#include "bench_common.h"

typedef float DATA_TYPE;

__global__ void atax_kernel1(DATA_TYPE *A, DATA_TYPE *x, DATA_TYPE *tmp, int nx, int ny) {
    int i = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    if (i < nx) {
        int j;
        for (j = 0; j < ny; j++) {
            tmp[i] += A[i * ny + j] * x[j];
        }
    }
}

__global__ void atax_kernel2(DATA_TYPE *A, DATA_TYPE *y, DATA_TYPE *tmp, int nx, int ny) {
    int j = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    if (j < ny) {
        int i;
        for (i = 0; i < nx; i++) {
            y[j] += A[i * ny + j] * tmp[i];
        }
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
    HIP_CHECK(hipMalloc(&d_A, NX * NY * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_x, NY * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_tmp, NX * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_y, NY * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_A, h_A, NX * NY * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_x, h_x, NY * sizeof(DATA_TYPE), hipMemcpyHostToDevice));

    const int THREADS = 256;
    dim3 block(THREADS);
    dim3 grid1((NX + THREADS - 1) / THREADS);
    dim3 grid2((NY + THREADS - 1) / THREADS);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", NX, NY);

    BenchResult r = runBenchmark("atax", problemSize, iterations, [&]() {
        // Reset tmp and y each iteration
        HIP_CHECK(hipMemset(d_tmp, 0, NX * sizeof(DATA_TYPE)));
        HIP_CHECK(hipMemset(d_y, 0, NY * sizeof(DATA_TYPE)));
        atax_kernel1<<<grid1, block>>>(d_A, d_x, d_tmp, NX, NY);
        atax_kernel2<<<grid2, block>>>(d_A, d_y, d_tmp, NX, NY);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_x));
    HIP_CHECK(hipFree(d_tmp));
    HIP_CHECK(hipFree(d_y));
    free(h_A);
    free(h_x);

    return 0;
}
