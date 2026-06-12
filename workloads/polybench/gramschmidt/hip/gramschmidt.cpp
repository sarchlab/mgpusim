// gramschmidt.cpp -- HIP benchmark for PolyBench Gram-Schmidt
// QR factorization via modified Gram-Schmidt orthogonalization
// Input: M×N matrix A, output Q (orthogonal) and R (upper triangular)
// Sequential over columns, parallel within each column operation

#include "bench_common_hip.h"

typedef float DATA_TYPE;

// Kernel 1: Compute squared norm of column k of Q (partial reduction via atomicAdd)
__global__ void gs_norm_kernel(DATA_TYPE *Q, DATA_TYPE *R, int M, int N, int k) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    // Use shared memory for block-level reduction
    extern __shared__ DATA_TYPE sdata[];
    DATA_TYPE val = 0.0f;
    if (i < M) {
        val = Q[i * N + k] * Q[i * N + k];
    }
    sdata[hipThreadIdx_x] = val;
    __syncthreads();

    // Reduction in shared memory
    for (int s = hipBlockDim_x / 2; s > 0; s >>= 1) {
        if ((int)hipThreadIdx_x < s) {
            sdata[hipThreadIdx_x] += sdata[hipThreadIdx_x + s];
        }
        __syncthreads();
    }

    if (hipThreadIdx_x == 0) {
        atomicAdd(&R[k * N + k], sdata[0]);
    }
}

// Kernel 1b: Take square root of R[k][k]
__global__ void gs_sqrt_kernel(DATA_TYPE *R, int N, int k) {
    R[k * N + k] = sqrtf(R[k * N + k]);
}

// Kernel 2: Normalize column k: Q[:,k] /= R[k][k]
__global__ void gs_normalize_kernel(DATA_TYPE *Q, DATA_TYPE *R, int M, int N, int k) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (i < M) {
        Q[i * N + k] /= R[k * N + k];
    }
}

// Kernel 3: For column j > k, compute R[k][j] = dot(Q[:,k], Q[:,j]) via atomicAdd
__global__ void gs_dot_kernel(DATA_TYPE *Q, DATA_TYPE *R, int M, int N, int k, int j) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    extern __shared__ DATA_TYPE sdata[];
    DATA_TYPE val = 0.0f;
    if (i < M) {
        val = Q[i * N + k] * Q[i * N + j];
    }
    sdata[hipThreadIdx_x] = val;
    __syncthreads();

    for (int s = hipBlockDim_x / 2; s > 0; s >>= 1) {
        if ((int)hipThreadIdx_x < s) {
            sdata[hipThreadIdx_x] += sdata[hipThreadIdx_x + s];
        }
        __syncthreads();
    }

    if (hipThreadIdx_x == 0) {
        atomicAdd(&R[k * N + j], sdata[0]);
    }
}

// Kernel 4: Update column j: Q[:,j] -= R[k][j] * Q[:,k]
__global__ void gs_update_kernel(DATA_TYPE *Q, DATA_TYPE *R, int M, int N, int k, int j) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (i < M) {
        Q[i * N + j] -= R[k * N + j] * Q[i * N + k];
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int N = parseIntParam(argc, argv, "--size", 256);
    int M = parseIntParam(argc, argv, "--rows", 256);

    // Host allocations -- A is M×N, Q is M×N, R is N×N
    DATA_TYPE* h_A = (DATA_TYPE*)malloc(M * N * sizeof(DATA_TYPE));
    DATA_TYPE* h_Q = (DATA_TYPE*)malloc(M * N * sizeof(DATA_TYPE));
    DATA_TYPE* h_R = (DATA_TYPE*)malloc(N * N * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < M * N; i++) h_A[i] = (DATA_TYPE)(rand() % 100) / 10.0f;
    // Initialize Q = A
    for (int i = 0; i < M * N; i++) h_Q[i] = h_A[i];
    for (int i = 0; i < N * N; i++) h_R[i] = 0.0f;

    // Device allocations
    DATA_TYPE *d_Q, *d_R;
    HIP_CHECK(hipMalloc(&d_Q, M * N * sizeof(DATA_TYPE)));
    HIP_CHECK(hipMalloc(&d_R, N * N * sizeof(DATA_TYPE)));

    int blockSize = 256;
    int gridM = (M + blockSize - 1) / blockSize;
    size_t smemSize = blockSize * sizeof(DATA_TYPE);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();
    BenchResult r = runBenchmark("gramschmidt", problemSize, iterations, [&]() {
        // Re-upload Q and R each iteration for consistent results
        HIP_CHECK(hipMemcpy(d_Q, h_Q, M * N * sizeof(DATA_TYPE), hipMemcpyHostToDevice));
        HIP_CHECK(hipMemcpy(d_R, h_R, N * N * sizeof(DATA_TYPE), hipMemcpyHostToDevice));

        for (int k = 0; k < N; k++) {
            // Zero R[k][k] before reduction
            DATA_TYPE zero = 0.0f;
            HIP_CHECK(hipMemcpy(&d_R[k * N + k], &zero, sizeof(DATA_TYPE), hipMemcpyHostToDevice));

            // Compute norm
            gs_norm_kernel<<<gridM, blockSize, smemSize>>>(d_Q, d_R, M, N, k);
            gs_sqrt_kernel<<<1, 1>>>(d_R, N, k);

            // Normalize
            gs_normalize_kernel<<<gridM, blockSize>>>(d_Q, d_R, M, N, k);

            // Orthogonalize remaining columns
            for (int j = k + 1; j < N; j++) {
                DATA_TYPE zero_val = 0.0f;
                HIP_CHECK(hipMemcpy(&d_R[k * N + j], &zero_val, sizeof(DATA_TYPE), hipMemcpyHostToDevice));
                gs_dot_kernel<<<gridM, blockSize, smemSize>>>(d_Q, d_R, M, N, k, j);
                gs_update_kernel<<<gridM, blockSize>>>(d_Q, d_R, M, N, k, j);
            }
        }
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_Q));
    HIP_CHECK(hipFree(d_R));
    free(h_A); free(h_Q); free(h_R);

    return 0;
}
