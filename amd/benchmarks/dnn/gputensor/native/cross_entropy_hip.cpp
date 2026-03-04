/**
 * cross_entropy_hip.cpp: HIP implementation of cross entropy kernels
 * Translated from cross_entropy.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

extern "C" __global__ void cross_entropy_derivative(float *output,
                                       const float *input,
                                       const int *label,
                                       int batch_size, int num_per_image) {
  int gid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  int batch = gid / num_per_image;
  int elem = gid % num_per_image;

  if (label[batch] == elem) {
    output[gid] = -1 / input[gid];
  } else {
    output[gid] = 0;
  }
}

extern "C" __global__ void softmax_cross_entropy_derivative(float *output,
                                               const float *input,
                                               const int *label,
                                               int batch_size,
                                               int num_per_image) {
  int gid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  int batch = gid / num_per_image;
  int elem = gid % num_per_image;

  if (label[batch] == elem) {
    output[gid] = input[gid] - 1;
  } else {
    output[gid] = input[gid];
  }
}
