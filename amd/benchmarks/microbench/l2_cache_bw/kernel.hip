#include <hip/hip_runtime.h>

// L2 cache bandwidth: repeated reads from a medium buffer (fits in L2, not L1).
// The simulator sample treats N as output elements/threads, so:
//   total_read_bytes = N * numRepeats * sizeof(float)
// bufSize is the non-scaling cache-residency footprint.
__global__ void l2_cache_bw_kernel(const float* __restrict__ A,
                                   float* __restrict__ B,
                                   int N,
                                   int bufSize,
                                   int numRepeats) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float sum = 0.0f;
        for (int r = 0; r < numRepeats; r++) {
            int idx = (gid + r * 64) % bufSize;
            sum += A[idx];
        }
        B[gid] = sum;
    }
}
