#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

#define FLOAT_N 3214212.01f
#define EPS     0.005f

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

__global__ void stddev_kernel(DATA_TYPE *data, DATA_TYPE *mean, DATA_TYPE *stddev,
                              int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (j < M) {
        DATA_TYPE sum = 0.0f;
        for (int i = 0; i < N; i++) {
            DATA_TYPE diff = data[i * M + j] - mean[j];
            sum += diff * diff;
        }
        stddev[j] = sqrtf(sum / (DATA_TYPE)N);
        if (stddev[j] < EPS) stddev[j] = 1.0f;
    }
}

__global__ void normalize_kernel(DATA_TYPE *data, DATA_TYPE *mean, DATA_TYPE *stddev,
                                 int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < N && j < M) {
        data[i * M + j] = (data[i * M + j] - mean[j]) / (sqrtf((DATA_TYPE)N) * stddev[j]);
    }
}

__global__ void corr_kernel(DATA_TYPE *data, DATA_TYPE *corr, int N, int M) {
    int j1 = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < M && j1 < M) {
        if (i == j1) {
            corr[i * M + j1] = 1.0f;
        } else if (i < j1) {
            DATA_TYPE sum = 0.0f;
            for (int k = 0; k < N; k++) {
                sum += data[k * M + i] * data[k * M + j1];
            }
            corr[i * M + j1] = sum;
            corr[j1 * M + i] = sum;
        }
    }
}
