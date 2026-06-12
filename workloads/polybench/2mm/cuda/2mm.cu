// 2mm.cu -- CUDA benchmark for PolyBench 2MM
// D = alpha*A*B*C + beta*D
// Two kernels: mm2_kernel1 computes tmp=alpha*A*B, mm2_kernel2 computes D=tmp*C+beta*D

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

__global__ void mm2_kernel1(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *C,
                            DATA_TYPE alpha, int NI, int NK, int NJ) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < NI && j < NJ) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NK; k++) {
            sum += alpha * A[i * NK + k] * B[k * NJ + j];
        }
        C[i * NJ + j] = sum;
    }
}

__global__ void mm2_kernel2(DATA_TYPE *C, DATA_TYPE *D_in, DATA_TYPE *D_out,
                            DATA_TYPE beta, int NI, int NJ, int NL) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < NI && j < NL) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NJ; k++) {
            sum += C[i * NJ + k] * D_in[k * NL + j];
        }
        D_out[i * NL + j] = sum + beta * D_out[i * NL + j];
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    int NI = N, NJ = N, NK = N, NL = N;
    DATA_TYPE alpha = 32412.0f, beta = 2123.0f;

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(NI * NK * sizeof(DATA_TYPE));
    DATA_TYPE* h_B = (DATA_TYPE*)malloc(NK * NJ * sizeof(DATA_TYPE));
    DATA_TYPE* h_C = (DATA_TYPE*)malloc(NJ * NL * sizeof(DATA_TYPE));
    DATA_TYPE* h_D = (DATA_TYPE*)malloc(NI * NL * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < NI * NK; i++) h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NK * NJ; i++) h_B[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NJ * NL; i++) h_C[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NI * NL; i++) h_D[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_B, *d_tmp, *d_C, *d_D;
    CUDA_CHECK(cudaMalloc(&d_A, NI * NK * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_B, NK * NJ * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_tmp, NI * NJ * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_C, NJ * NL * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_D, NI * NL * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_A, h_A, NI * NK * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_B, h_B, NK * NJ * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_C, h_C, NJ * NL * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_D, h_D, NI * NL * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));

    dim3 block(16, 16);
    dim3 grid1((NJ + block.x - 1) / block.x, (NI + block.y - 1) / block.y);
    dim3 grid2((NL + block.x - 1) / block.x, (NI + block.y - 1) / block.y);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();
    BenchResult r = runBenchmark("2mm", problemSize, iterations, [&]() {
        mm2_kernel1<<<grid1, block>>>(d_A, d_B, d_tmp, alpha, NI, NK, NJ);
        mm2_kernel2<<<grid2, block>>>(d_tmp, d_C, d_D, beta, NI, NJ, NL);
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_A));
    CUDA_CHECK(cudaFree(d_B));
    CUDA_CHECK(cudaFree(d_tmp));
    CUDA_CHECK(cudaFree(d_C));
    CUDA_CHECK(cudaFree(d_D));
    free(h_A); free(h_B); free(h_C); free(h_D);

    return 0;
}
