#include <hip/hip_runtime.h>

// ---------------------------------------------------------------------------
// Kernel: shared-memory tree reduction
// Each block reduces 2*blockDim.x elements to one partial sum.
// ---------------------------------------------------------------------------
__global__ void reduce_kernel(const float* __restrict__ input,
                              float* __restrict__ output,
                              int N) {
    static __shared__ float sdata[1024];

    unsigned int tid = threadIdx.x;
    unsigned int i = blockIdx.x * blockDim.x * 2 + threadIdx.x;

    // Load two elements per thread into shared memory
    float val = 0.0f;
    if (i < N)
        val = input[i];
    if (i + blockDim.x < N)
        val += input[i + blockDim.x];
    sdata[tid] = val;
    __syncthreads();

    // Tree reduction in shared memory
    for (unsigned int s = blockDim.x / 2; s > 0; s >>= 1) {
        if (tid < s) {
            sdata[tid] += sdata[tid + s];
        }
        __syncthreads();
    }

    // Write result for this block
    if (tid == 0) {
        output[blockIdx.x] = sdata[0];
    }
}
