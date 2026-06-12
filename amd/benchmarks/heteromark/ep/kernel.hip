#include <hip/hip_runtime.h>

#define MAX_BLOCK_SIZE 512

// ---------------------------------------------------------------------------
// Kernel: Per-block exclusive Blelloch scan
// ---------------------------------------------------------------------------
__global__ void scan_block(const float* __restrict__ input,
                           float* __restrict__ output,
                           float* __restrict__ blockSums,
                           int N) {
    static __shared__ float temp[1024];

    int tid = hipThreadIdx_x;
    int blockOffset = hipBlockIdx_x * hipBlockDim_x * 2;
    int ai = tid;
    int bi = tid + hipBlockDim_x;

    // Load input into shared memory
    temp[ai] = (blockOffset + ai < N) ? input[blockOffset + ai] : 0.0f;
    temp[bi] = (blockOffset + bi < N) ? input[blockOffset + bi] : 0.0f;

    int n = hipBlockDim_x * 2;

    // Up-sweep (reduce) phase
    int offset = 1;
    for (int d = n >> 1; d > 0; d >>= 1) {
        __syncthreads();
        if (tid < d) {
            int ai2 = offset * (2 * tid + 1) - 1;
            int bi2 = offset * (2 * tid + 2) - 1;
            temp[bi2] += temp[ai2];
        }
        offset <<= 1;
    }

    // Save block sum and clear last element
    __syncthreads();
    if (tid == 0) {
        if (blockSums != nullptr) {
            blockSums[hipBlockIdx_x] = temp[n - 1];
        }
        temp[n - 1] = 0.0f;
    }

    // Down-sweep phase
    for (int d = 1; d < n; d <<= 1) {
        offset >>= 1;
        __syncthreads();
        if (tid < d) {
            int ai2 = offset * (2 * tid + 1) - 1;
            int bi2 = offset * (2 * tid + 2) - 1;
            float t = temp[ai2];
            temp[ai2] = temp[bi2];
            temp[bi2] += t;
        }
    }

    __syncthreads();

    // Write results
    if (blockOffset + ai < N) output[blockOffset + ai] = temp[ai];
    if (blockOffset + bi < N) output[blockOffset + bi] = temp[bi];
}

// ---------------------------------------------------------------------------
// Kernel: Add block prefix sums back to produce final exclusive scan
// ---------------------------------------------------------------------------
__global__ void add_block_prefix(float* __restrict__ data,
                                 const float* __restrict__ blockPrefix,
                                 int N,
                                 int elemsPerBlock) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (i < N) {
        int origBlock = i / elemsPerBlock;
        if (origBlock > 0) {
            data[i] += blockPrefix[origBlock];
        }
    }
}
