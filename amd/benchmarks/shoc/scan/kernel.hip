#include <hip/hip_runtime.h>

#define MAX_BLOCK_SIZE 512

// ---------------------------------------------------------------------------
// Shared-memory work-efficient scan (exclusive) for one block
// ---------------------------------------------------------------------------
__global__ void scan_block_kernel(float* __restrict__ output,
                                  const float* __restrict__ input,
                                  float* __restrict__ block_sums,
                                  int N) {
    static __shared__ float temp[1024];

    int tid = threadIdx.x;
    int block_offset = blockIdx.x * (blockDim.x * 2);
    int ai = tid;
    int bi = tid + blockDim.x;

    // Load input into shared memory
    temp[ai] = (block_offset + ai < N) ? input[block_offset + ai] : 0.0f;
    temp[bi] = (block_offset + bi < N) ? input[block_offset + bi] : 0.0f;

    int n = blockDim.x * 2;

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
        if (block_sums != nullptr) {
            block_sums[blockIdx.x] = temp[n - 1];
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
    if (block_offset + ai < N) output[block_offset + ai] = temp[ai];
    if (block_offset + bi < N) output[block_offset + bi] = temp[bi];
}

// ---------------------------------------------------------------------------
// Add block sums back to scanned blocks
// ---------------------------------------------------------------------------
__global__ void add_block_sums_kernel(float* __restrict__ data,
                                      const float* __restrict__ block_sums,
                                      int N) {
    int idx = blockIdx.x * (blockDim.x * 2) + threadIdx.x;
    if (blockIdx.x > 0) {
        float sum = block_sums[blockIdx.x];
        if (idx < N) data[idx] += sum;
        idx += blockDim.x;
        if (idx < N) data[idx] += sum;
    }
}
