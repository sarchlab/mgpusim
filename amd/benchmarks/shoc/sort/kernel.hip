#include <hip/hip_runtime.h>

#define MAX_BLOCK_SIZE 512

// ---------------------------------------------------------------------------
// Scan kernel (Blelloch-style work-efficient exclusive prefix sum)
// ---------------------------------------------------------------------------
__global__ void scan_block_kernel(unsigned int* __restrict__ output,
                                  const unsigned int* __restrict__ input,
                                  unsigned int* __restrict__ block_sums,
                                  int N) {
    static __shared__ unsigned int stemp[1024];

    int tid = threadIdx.x;
    int block_offset = blockIdx.x * (blockDim.x * 2);
    int ai = tid;
    int bi = tid + blockDim.x;

    stemp[ai] = (block_offset + ai < N) ? input[block_offset + ai] : 0;
    stemp[bi] = (block_offset + bi < N) ? input[block_offset + bi] : 0;

    int n = blockDim.x * 2;

    // Up-sweep
    int offset = 1;
    for (int d = n >> 1; d > 0; d >>= 1) {
        __syncthreads();
        if (tid < d) {
            int ai2 = offset * (2 * tid + 1) - 1;
            int bi2 = offset * (2 * tid + 2) - 1;
            stemp[bi2] += stemp[ai2];
        }
        offset <<= 1;
    }

    __syncthreads();
    if (tid == 0) {
        if (block_sums != nullptr) {
            block_sums[blockIdx.x] = stemp[n - 1];
        }
        stemp[n - 1] = 0;
    }

    // Down-sweep
    for (int d = 1; d < n; d <<= 1) {
        offset >>= 1;
        __syncthreads();
        if (tid < d) {
            int ai2 = offset * (2 * tid + 1) - 1;
            int bi2 = offset * (2 * tid + 2) - 1;
            unsigned int t = stemp[ai2];
            stemp[ai2] = stemp[bi2];
            stemp[bi2] += t;
        }
    }

    __syncthreads();
    if (block_offset + ai < N) output[block_offset + ai] = stemp[ai];
    if (block_offset + bi < N) output[block_offset + bi] = stemp[bi];
}

__global__ void add_block_sums_uint_kernel(unsigned int* __restrict__ data,
                                            const unsigned int* __restrict__ block_sums,
                                            int N) {
    int idx = blockIdx.x * (blockDim.x * 2) + threadIdx.x;
    if (blockIdx.x > 0) {
        unsigned int sum = block_sums[blockIdx.x];
        if (idx < N) data[idx] += sum;
        idx += blockDim.x;
        if (idx < N) data[idx] += sum;
    }
}

// ---------------------------------------------------------------------------
// Radix sort kernels
// ---------------------------------------------------------------------------

// Compute predicate: is bit b of keys[i] == 0?
__global__ void compute_predicates_kernel(unsigned int* __restrict__ predicates,
                                          const unsigned int* __restrict__ keys,
                                          int bit, int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < N) {
        predicates[i] = ((keys[i] >> bit) & 1u) == 0u ? 1u : 0u;
    }
}

// Scatter elements based on scan results
__global__ void scatter_kernel(unsigned int* __restrict__ keys_out,
                               const unsigned int* __restrict__ keys_in,
                               const unsigned int* __restrict__ predicates,
                               const unsigned int* __restrict__ scan_result,
                               unsigned int total_zeros, int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < N) {
        unsigned int key = keys_in[i];
        unsigned int dest;
        if (predicates[i]) {
            // Bit is 0 -- goes to front
            dest = scan_result[i];
        } else {
            // Bit is 1 -- goes after all zeros
            dest = total_zeros + (i - scan_result[i]);
        }
        keys_out[dest] = key;
    }
}
