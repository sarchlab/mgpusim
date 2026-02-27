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
