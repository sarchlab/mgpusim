/*
 * HIP kernel for FIR filter (gfx942 / CDNA3)
 * Translated from kernels.cl
 *
 * Finite Impulse Response filter.
 * Each thread computes one output sample as a weighted sum of input samples.
 */
#include "hip/hip_runtime.h"

extern "C" __global__ void FIR(
    float* output,
    float* coeff,
    float* input,
    float* history,
    unsigned int num_tap)
{
    unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    unsigned int num_data = hipGridDim_x * hipBlockDim_x;

    float sum = 0.0f;
    for (unsigned int i = 0; i < num_tap; i++) {
        if (tid >= i) {
            sum = sum + coeff[i] * input[tid - i];
        } else {
            sum = sum + coeff[i] * history[num_tap - (i - tid)];
        }
    }
    output[tid] = sum;
}


// GPU Benchmarks v1.1, heteromark_fir/hip/heteromark_fir.hip.
#define NUM_TAPS 128
extern "C" __global__ void fir_filter_kernel(const float* __restrict__ input,
                                   float* __restrict__ output,
                                   const float* __restrict__ coeff,
                                   int num_samples,
                                   int num_taps) {
    __shared__ float s_coeff[NUM_TAPS];

    // Cooperatively load coefficients into shared memory
    for (int t = threadIdx.x; t < num_taps; t += blockDim.x) {
        s_coeff[t] = coeff[t];
    }
    __syncthreads();

    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx >= num_samples) return;

    float sum = 0.0f;
    for (int k = 0; k < num_taps; ++k) {
        int in_idx = idx - k;
        if (in_idx >= 0) {
            sum += s_coeff[k] * input[in_idx];
        }
    }

    output[idx] = sum;
}
