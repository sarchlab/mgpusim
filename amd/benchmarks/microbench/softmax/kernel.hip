#include <hip/hip_runtime.h>

// Row-wise softmax: each workgroup processes one row of length cols.
// blockDim.x threads cooperate via shared memory reduction.
// Max blockDim.x = 256 (matching HW bs256 configuration)
#define MAX_BLOCK_SIZE 256

__global__ void softmax_kernel(const float* __restrict__ A,
                               float* __restrict__ B,
                               int rows,
                               int cols) {
    __shared__ float sdata[MAX_BLOCK_SIZE];

    int row = blockIdx.x;
    if (row >= rows) return;

    int tid = threadIdx.x;
    int blockSize = blockDim.x;

    const float* rowIn = A + row * cols;
    float* rowOut = B + row * cols;

    // Phase 1: find row max
    float localMax = -1e30f;
    for (int j = tid; j < cols; j += blockSize) {
        float v = rowIn[j];
        if (v > localMax) localMax = v;
    }
    sdata[tid] = localMax;
    __syncthreads();

    for (int s = blockSize / 2; s > 0; s >>= 1) {
        if (tid < s) {
            if (sdata[tid + s] > sdata[tid])
                sdata[tid] = sdata[tid + s];
        }
        __syncthreads();
    }
    float rowMax = sdata[0];
    __syncthreads();

    // Phase 2: compute exp(x - max) and sum
    float localSum = 0.0f;
    for (int j = tid; j < cols; j += blockSize) {
        float v = expf(rowIn[j] - rowMax);
        rowOut[j] = v;
        localSum += v;
    }
    sdata[tid] = localSum;
    __syncthreads();

    for (int s = blockSize / 2; s > 0; s >>= 1) {
        if (tid < s) {
            sdata[tid] += sdata[tid + s];
        }
        __syncthreads();
    }
    float rowSum = sdata[0];
    __syncthreads();

    // Phase 3: normalize
    float invSum = 1.0f / rowSum;
    for (int j = tid; j < cols; j += blockSize) {
        rowOut[j] *= invSum;
    }
}
