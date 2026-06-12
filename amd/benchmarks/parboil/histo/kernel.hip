#include <hip/hip_runtime.h>

// Kernel: each thread processes one element, atomically incrementing the bin
__global__ void histogram(unsigned int *input, unsigned int *bins,
                          int num_elements, int num_bins,
                          unsigned int saturation) {
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (gid >= num_elements) return;

    unsigned int bin = input[gid] % num_bins;
    unsigned int old = atomicAdd(&bins[bin], 1u);

    // Saturation: cap the bin count at the saturation value
    if (saturation > 0 && old >= saturation) {
        atomicSub(&bins[bin], 1u);
    }
}

// Kernel: clear histogram bins to zero
__global__ void clear_bins(unsigned int *bins, int num_bins) {
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (gid < num_bins) {
        bins[gid] = 0;
    }
}
