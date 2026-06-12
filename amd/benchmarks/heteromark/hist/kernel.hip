#include <hip/hip_runtime.h>

#define DEFAULT_SIZE    1048576
#define DEFAULT_BINS    256
#define DEFAULT_BLOCK   256

// ---------------------------------------------------------------------------
// Kernel: Histogram with shared memory per-block accumulation
// ---------------------------------------------------------------------------
__global__ void histogram_kernel(const unsigned int* __restrict__ data,
                                 unsigned int* __restrict__ hist,
                                 int N,
                                 int numBins) {
    static __shared__ unsigned int localHist[1024];

    int tid = hipThreadIdx_x;
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int stride = hipBlockDim_x * hipGridDim_x;

    // Zero shared memory histogram
    for (int i = tid; i < numBins; i += hipBlockDim_x) {
        localHist[i] = 0;
    }
    __syncthreads();

    // Accumulate into shared memory histogram
    for (int i = gid; i < N; i += stride) {
        atomicAdd(&localHist[data[i]], 1);
    }
    __syncthreads();

    // Add shared histogram to global histogram
    for (int i = tid; i < numBins; i += hipBlockDim_x) {
        if (localHist[i] > 0) {
            atomicAdd(&hist[i], localHist[i]);
        }
    }
}
