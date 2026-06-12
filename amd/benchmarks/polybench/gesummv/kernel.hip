#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

__global__ void gesummv_kernel(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *x,
                               DATA_TYPE *y, DATA_TYPE alpha, DATA_TYPE beta,
                               int N) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (i < N) {
        DATA_TYPE tmp1 = 0.0f;
        DATA_TYPE tmp2 = 0.0f;
        for (int j = 0; j < N; j++) {
            tmp1 += A[i * N + j] * x[j];
            tmp2 += B[i * N + j] * x[j];
        }
        y[i] = alpha * tmp1 + beta * tmp2;
    }
}
