// bicg.cpp — HIP benchmark for PolyBench BiCG
// Kernels copied from amd/benchmarks/polybench/bicg/native/bicg.cpp
// Problem size: nx=256, ny=256

#include "bench_common.h"

typedef float DATA_TYPE;

__global__ void bicgKernel1(DATA_TYPE *A, DATA_TYPE *p, DATA_TYPE *q, int nx, int ny) {
    int i = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    if (i < nx) {
        q[i] = 0.0;

        int j;
        for (j = 0; j < ny; j++) {
            q[i] += A[i * ny + j] * p[j];
        }
    }
}

__global__ void bicgKernel2(DATA_TYPE *A, DATA_TYPE *r, DATA_TYPE *s, int nx, int ny) {
    int j = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    if (j < ny) {
        s[j] = 0.0;

        int i;
        for (i = 0; i < nx; i++) {
            s[j] += A[i * ny + j] * r[i];
        }
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);

    const int NX = 256;
    const int NY = 256;

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
    HIP_CHECK(hipMalloc(&d_A, NX * NY * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_p, NY * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_q, NX * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_r, NX * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_s, NY * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_A, h_A, NX * NY * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_p, h_p, NY * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_r, h_r, NX * sizeof(DATA_TYPE), hipMemcpyHostToDevice));

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
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_p));
    HIP_CHECK(hipFree(d_q));
    HIP_CHECK(hipFree(d_r));
    HIP_CHECK(hipFree(d_s));
    free(h_A);
    free(h_p);
    free(h_r);

    return 0;
}
