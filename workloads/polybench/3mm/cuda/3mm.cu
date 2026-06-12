// 3mm.cu -- CUDA benchmark for PolyBench 3MM
// E = A*B, F = C*D, G = E*F
// Three kernels: mm3_kernel1, mm3_kernel2, mm3_kernel3

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

__global__ void mm3_kernel1(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *E,
                            int NI, int NK, int NJ) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < NI && j < NJ) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NK; k++) {
            sum += A[i * NK + k] * B[k * NJ + j];
        }
        E[i * NJ + j] = sum;
    }
}

__global__ void mm3_kernel2(DATA_TYPE *C, DATA_TYPE *D, DATA_TYPE *F,
                            int NJ, int NM, int NL) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < NJ && j < NL) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NM; k++) {
            sum += C[i * NM + k] * D[k * NL + j];
        }
        F[i * NL + j] = sum;
    }
}

__global__ void mm3_kernel3(DATA_TYPE *E, DATA_TYPE *F, DATA_TYPE *G,
                            int NI, int NJ, int NL) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < NI && j < NL) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NJ; k++) {
            sum += E[i * NJ + k] * F[k * NL + j];
        }
        G[i * NL + j] = sum;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    int NI = N, NJ = N, NK = N, NL = N, NM = N;

    // Host allocations
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(NI * NK * sizeof(DATA_TYPE));
    DATA_TYPE* h_B = (DATA_TYPE*)malloc(NK * NJ * sizeof(DATA_TYPE));
    DATA_TYPE* h_C = (DATA_TYPE*)malloc(NJ * NM * sizeof(DATA_TYPE));
    DATA_TYPE* h_D = (DATA_TYPE*)malloc(NM * NL * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < NI * NK; i++) h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NK * NJ; i++) h_B[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NJ * NM; i++) h_C[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NM * NL; i++) h_D[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_A, *d_B, *d_C, *d_D, *d_E, *d_F, *d_G;
    CUDA_CHECK(cudaMalloc(&d_A, NI * NK * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_B, NK * NJ * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_C, NJ * NM * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_D, NM * NL * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_E, NI * NJ * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_F, NJ * NL * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_G, NI * NL * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_A, h_A, NI * NK * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_B, h_B, NK * NJ * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_C, h_C, NJ * NM * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_D, h_D, NM * NL * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));

    dim3 block(16, 16);
    dim3 grid1((NJ + block.x - 1) / block.x, (NI + block.y - 1) / block.y);
    dim3 grid2((NL + block.x - 1) / block.x, (NJ + block.y - 1) / block.y);
    dim3 grid3((NL + block.x - 1) / block.x, (NI + block.y - 1) / block.y);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();
    BenchResult r = runBenchmark("3mm", problemSize, iterations, [&]() {
        mm3_kernel1<<<grid1, block>>>(d_A, d_B, d_E, NI, NK, NJ);
        mm3_kernel2<<<grid2, block>>>(d_C, d_D, d_F, NJ, NM, NL);
        mm3_kernel3<<<grid3, block>>>(d_E, d_F, d_G, NI, NJ, NL);
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_A));
    CUDA_CHECK(cudaFree(d_B));
    CUDA_CHECK(cudaFree(d_C));
    CUDA_CHECK(cudaFree(d_D));
    CUDA_CHECK(cudaFree(d_E));
    CUDA_CHECK(cudaFree(d_F));
    CUDA_CHECK(cudaFree(d_G));
    free(h_A); free(h_B); free(h_C); free(h_D);

    return 0;
}
