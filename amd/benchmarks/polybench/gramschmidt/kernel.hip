#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

// Kernel 1: Compute squared norm of column k of Q (partial reduction via atomicAdd)
__global__ void gs_norm_kernel(DATA_TYPE *Q, DATA_TYPE *R, int M, int N, int k) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    // Use shared memory for block-level reduction
    static __shared__ DATA_TYPE sdata[1024];
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

    static __shared__ DATA_TYPE sdata[1024];
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
