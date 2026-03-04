/**
 * repeat_hip.cpp: HIP implementation of repeat kernel
 * Translated from repeat.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

extern "C" __global__ void repeat(float *output, float *input,
                     const unsigned int input_length,
                     const unsigned int output_length) {
  unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  if (tid > output_length) {
    return;
  }

  unsigned int input_index = tid % input_length;

  output[tid] = input[input_index];
}
