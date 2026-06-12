#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

__global__ void syrk_kernel(DATA_TYPE *A, DATA_TYPE *C,
                            DATA_TYPE alpha, DATA_TYPE beta,
                            int N, int M) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < N && j <= i) {
        C[i * N + j] *= beta;
        for (int k = 0; k < M; k++) {
            C[i * N + j] += alpha * A[i * M + k] * A[j * M + k];
        }
    }
}
