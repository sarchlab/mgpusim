#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

__global__ void mvt_kernel1(DATA_TYPE *A, DATA_TYPE *x1, DATA_TYPE *y1,
                            int N) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (i < N) {
        DATA_TYPE sum = 0.0f;
        for (int j = 0; j < N; j++) {
            sum += A[i * N + j] * y1[j];
        }
        x1[i] += sum;
    }
}

__global__ void mvt_kernel2(DATA_TYPE *A, DATA_TYPE *x2, DATA_TYPE *y2,
                            int N) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (i < N) {
        DATA_TYPE sum = 0.0f;
        for (int j = 0; j < N; j++) {
            sum += A[j * N + i] * y2[j];
        }
        x2[i] += sum;
    }
}
