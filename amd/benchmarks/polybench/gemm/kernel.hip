#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

__global__ void gemm_kernel(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *C,
                            DATA_TYPE alpha, DATA_TYPE beta,
                            int NI, int NJ, int NK) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NI && j < NJ) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NK; k++) {
            sum += A[i * NK + k] * B[k * NJ + j];
        }
        C[i * NJ + j] = alpha * sum + beta * C[i * NJ + j];
    }
}
