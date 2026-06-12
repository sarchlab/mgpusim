#include <hip/hip_runtime.h>

// L1 cache bandwidth: repeated reads with a work-amount problem-size axis.
// The simulator sample treats N as output elements/threads, so:
//   total_read_bytes = N * numRepeats * sizeof(float)
// Each 256-thread read group cycles over 256 float elements (1 KiB) to keep the
// per-group access footprint L1-resident while N changes the amount of work.
// The Go host allocates one input group per covered gid range, so this embedded
// simulator path is not a fixed-total working_set_size implementation.
__global__ void l1_cache_bw_kernel(const float* __restrict__ A,
                                   float* __restrict__ B,
                                   int N,
                                   int numRepeats) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float sum = 0.0f;
        int base = (gid / 256) * 256;
        for (int r = 0; r < numRepeats; r++) {
            int idx = base + (threadIdx.x + r) % 256;
            sum += A[idx];
        }
        B[gid] = sum;
    }
}
