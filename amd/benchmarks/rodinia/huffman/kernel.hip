#include <hip/hip_runtime.h>

// Kernel: compute byte frequency histogram using atomicAdd
__global__ void histogram_kernel(const unsigned char *data,
                                 unsigned int *hist, int data_size) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= data_size) return;

    atomicAdd(&hist[data[tid]], 1u);
}

// Kernel: encode each byte using a codebook stored in global memory
__global__ void encode_kernel(const unsigned char *data,
                              const unsigned int *codebook_codes,
                              const unsigned int *codebook_lengths,
                              unsigned int *output_codes,
                              unsigned int *output_lengths,
                              int data_size) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= data_size) return;

    unsigned char sym = data[tid];
    output_codes[tid] = codebook_codes[sym];
    output_lengths[tid] = codebook_lengths[sym];
}
