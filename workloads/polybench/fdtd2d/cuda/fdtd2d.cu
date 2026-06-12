// fdtd2d.cu -- CUDA benchmark for PolyBench FDTD-2D
// 2D Finite Difference Time Domain (electromagnetic simulation)
// 3 update kernels per timestep over NX×NY arrays

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

// Kernel 1: Update ey[0,j] from _fict_[t]
__global__ void fdtd2d_kernel1(DATA_TYPE *ey, DATA_TYPE *fict, int NY, int t) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;

    if (j < NY) {
        ey[0 * NY + j] = fict[t];
    }
}

// Kernel 2: Update ey[i,j] for i>0: ey[i][j] -= 0.5*(hz[i][j] - hz[i-1][j])
__global__ void fdtd2d_kernel2(DATA_TYPE *ey, DATA_TYPE *hz, int NX, int NY) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i >= 1 && i < NX && j < NY) {
        ey[i * NY + j] -= 0.5f * (hz[i * NY + j] - hz[(i - 1) * NY + j]);
    }
}

// Kernel 3: Update ex[i,j] for j>0: ex[i][j] -= 0.5*(hz[i][j] - hz[i][j-1])
__global__ void fdtd2d_kernel3(DATA_TYPE *ex, DATA_TYPE *hz, int NX, int NY) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < NX && j >= 1 && j < NY) {
        ex[i * NY + j] -= 0.5f * (hz[i * NY + j] - hz[i * NY + (j - 1)]);
    }
}

// Kernel 4: Update hz: hz[i][j] -= 0.7*(ex[i][j+1]-ex[i][j] + ey[i+1][j]-ey[i][j])
__global__ void fdtd2d_kernel4(DATA_TYPE *ex, DATA_TYPE *ey, DATA_TYPE *hz,
                               int NX, int NY) {
    int j = blockIdx.x * blockDim.x + threadIdx.x;
    int i = blockIdx.y * blockDim.y + threadIdx.y;

    if (i < NX - 1 && j < NY - 1) {
        hz[i * NY + j] -= 0.7f * (ex[i * NY + (j + 1)] - ex[i * NY + j] +
                                   ey[(i + 1) * NY + j] - ey[i * NY + j]);
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    int TMAX = parseIntParam(argc, argv, "--tmax", 10);
    int NX = N, NY = N;

    // Host allocations
    DATA_TYPE* h_ex   = (DATA_TYPE*)malloc(NX * NY * sizeof(DATA_TYPE));
    DATA_TYPE* h_ey   = (DATA_TYPE*)malloc(NX * NY * sizeof(DATA_TYPE));
    DATA_TYPE* h_hz   = (DATA_TYPE*)malloc(NX * NY * sizeof(DATA_TYPE));
    DATA_TYPE* h_fict = (DATA_TYPE*)malloc(TMAX * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < NX * NY; i++) h_ex[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NX * NY; i++) h_ey[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < NX * NY; i++) h_hz[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    for (int i = 0; i < TMAX; i++) h_fict[i] = (DATA_TYPE)(rand() % 100) / 10.0f;

    // Device allocations
    DATA_TYPE *d_ex, *d_ey, *d_hz, *d_fict;
    CUDA_CHECK(cudaMalloc(&d_ex,   NX * NY * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_ey,   NX * NY * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_hz,   NX * NY * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_fict, TMAX * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_ex,   h_ex,   NX * NY * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_ey,   h_ey,   NX * NY * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_hz,   h_hz,   NX * NY * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_fict, h_fict, TMAX * sizeof(DATA_TYPE), cudaMemcpyHostToDevice));

    dim3 block(16, 16);
    dim3 grid2d((NY + block.x - 1) / block.x, (NX + block.y - 1) / block.y);
    int block1d = 256;
    int grid1d = (NY + block1d - 1) / block1d;

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();
    BenchResult r = runBenchmark("fdtd2d", problemSize, iterations, [&]() {
        for (int t = 0; t < TMAX; t++) {
            fdtd2d_kernel1<<<grid1d, block1d>>>(d_ey, d_fict, NY, t);
            fdtd2d_kernel2<<<grid2d, block>>>(d_ey, d_hz, NX, NY);
            fdtd2d_kernel3<<<grid2d, block>>>(d_ex, d_hz, NX, NY);
            fdtd2d_kernel4<<<grid2d, block>>>(d_ex, d_ey, d_hz, NX, NY);
        }
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_ex));
    CUDA_CHECK(cudaFree(d_ey));
    CUDA_CHECK(cudaFree(d_hz));
    CUDA_CHECK(cudaFree(d_fict));
    free(h_ex); free(h_ey); free(h_hz); free(h_fict);

    return 0;
}
