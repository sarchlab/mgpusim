#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

__global__ void mm2_kernel1(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *C,
                            DATA_TYPE alpha, int NI, int NK, int NJ) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NI && j < NJ) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NK; k++) {
            sum += alpha * A[i * NK + k] * B[k * NJ + j];
        }
        C[i * NJ + j] = sum;
    }
}

__global__ void mm2_kernel2(DATA_TYPE *C, DATA_TYPE *D_in, DATA_TYPE *D_out,
                            DATA_TYPE beta, int NI, int NJ, int NL) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NI && j < NL) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NJ; k++) {
            sum += C[i * NJ + k] * D_in[k * NL + j];
        }
        D_out[i * NL + j] = sum + beta * D_out[i * NL + j];
    }
}
