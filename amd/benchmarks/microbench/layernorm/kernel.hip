#include <hip/hip_runtime.h>

// Layer normalization: each workgroup processes one row of length hid.
// y[i] = gamma[i] * (x[i] - mean) / sqrt(var + eps) + beta[i]
// Max blockDim.x = 256 (matching HW bs256 configuration)
#define MAX_BLOCK_SIZE 256

__global__ void layernorm_kernel(const float* __restrict__ A,
                                 float* __restrict__ B,
                                 const float* __restrict__ gamma,
                                 const float* __restrict__ beta,
                                 int rows,
                                 int hid) {
    __shared__ float sdata[MAX_BLOCK_SIZE];

    int row = blockIdx.x;
    if (row >= rows) return;

    int tid = threadIdx.x;
    int blockSize = blockDim.x;

    const float* rowIn = A + row * hid;
    float* rowOut = B + row * hid;

    // Phase 1: compute mean
    float localSum = 0.0f;
    for (int j = tid; j < hid; j += blockSize) {
        localSum += rowIn[j];
    }
    sdata[tid] = localSum;
    __syncthreads();

    for (int s = blockSize / 2; s > 0; s >>= 1) {
        if (tid < s) {
            sdata[tid] += sdata[tid + s];
        }
        __syncthreads();
    }
    float mean = sdata[0] / (float)hid;
    __syncthreads();

    // Phase 2: compute variance
    float localVar = 0.0f;
    for (int j = tid; j < hid; j += blockSize) {
        float diff = rowIn[j] - mean;
        localVar += diff * diff;
    }
    sdata[tid] = localVar;
    __syncthreads();

    for (int s = blockSize / 2; s > 0; s >>= 1) {
        if (tid < s) {
            sdata[tid] += sdata[tid + s];
        }
        __syncthreads();
    }
    float var = sdata[0] / (float)hid;
    float invStd = rsqrtf(var + 1e-5f);
    __syncthreads();

    // Phase 3: normalize
    for (int j = tid; j < hid; j += blockSize) {
        float norm = (rowIn[j] - mean) * invStd;
        rowOut[j] = gamma[j] * norm + beta[j];
    }
}
