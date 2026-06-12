#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

__global__ void mean_kernel(DATA_TYPE *data, DATA_TYPE *mean, int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (j < M) {
        DATA_TYPE sum = 0.0f;
        for (int i = 0; i < N; i++) {
            sum += data[i * M + j];
        }
        mean[j] = sum / (DATA_TYPE)N;
    }
}

__global__ void reduce_kernel(DATA_TYPE *data, DATA_TYPE *mean, int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < N && j < M) {
        data[i * M + j] -= mean[j];
    }
}

__global__ void covar_kernel(DATA_TYPE *data, DATA_TYPE *cov, int N, int M) {
    int j1 = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

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
