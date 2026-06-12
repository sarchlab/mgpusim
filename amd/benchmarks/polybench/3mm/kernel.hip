#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

__global__ void mm3_kernel1(DATA_TYPE *A, DATA_TYPE *B, DATA_TYPE *E,
                            int NI, int NK, int NJ) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NI && j < NJ) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NK; k++) {
            sum += A[i * NK + k] * B[k * NJ + j];
        }
        E[i * NJ + j] = sum;
    }
}

__global__ void mm3_kernel2(DATA_TYPE *C, DATA_TYPE *D, DATA_TYPE *F,
                            int NJ, int NM, int NL) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NJ && j < NL) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NM; k++) {
            sum += C[i * NM + k] * D[k * NL + j];
        }
        F[i * NL + j] = sum;
    }
}

__global__ void mm3_kernel3(DATA_TYPE *E, DATA_TYPE *F, DATA_TYPE *G,
                            int NI, int NJ, int NL) {
    int j = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int i = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (i < NI && j < NL) {
        DATA_TYPE sum = 0.0f;
        for (int k = 0; k < NJ; k++) {
            sum += E[i * NJ + k] * F[k * NL + j];
        }
        G[i * NL + j] = sum;
    }
}
