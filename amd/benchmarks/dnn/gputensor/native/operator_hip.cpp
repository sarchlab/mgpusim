/**
 * operator_hip.cpp: HIP implementation of DNN GPU tensor operator kernels
 * Translated from operator.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

__device__ int numElement(int* size, int dim) {
  int s = 1;
  for (int i = 0; i < dim; i++) {
    s *= size[i];
  }
  return s;
}

__device__ void unflatIndex(int* nd_index, int flat_index, int* size,
                 int dim) {
  int i;
  int total_size = numElement(size, dim);

  for (i = 0; i < dim; i++) {
    total_size /= size[i];
    int pos = flat_index / total_size;
    flat_index -= pos * total_size;
    nd_index[i] = pos;
  }
}

__device__ int flatIndex(int* nd_index, int* size, int dim) {
  int out = 0;
  int total_size = 1;

  for (int i = 0; i < dim; i++) {
    out += nd_index[dim - i - 1] * total_size;
    total_size *= size[dim - i - 1];
  }

  return out;
}

extern "C" __global__ void transpose_tensor(float* in, float* out,
                               int* in_size, int* out_size,
                               int* order, int* in_index_buf,
                               int* out_index_buf, const int dim) {
  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  int* nd_in_index = in_index_buf + tid * dim;
  int* nd_out_index = out_index_buf + tid * dim;

  unflatIndex(nd_out_index, tid, out_size, dim);

  for (int i = 0; i < dim; i++) {
    nd_in_index[order[i]] = nd_out_index[i];
  }

  int input_index_flat = flatIndex(nd_in_index, in_size, dim);

  out[tid] = in[input_index_flat];
}

extern "C" __global__ void rotate_tensor(float* in, float* out,
                            int* in_size, int* out_size,
                            int* in_index_buf,
                            int* out_index_buf, const int dim) {
  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  int* nd_in_index = in_index_buf + tid * dim;
  int* nd_out_index = out_index_buf + tid * dim;

  unflatIndex(nd_out_index, tid, out_size, dim);

  for (int i = 0; i < dim; i++) {
    nd_in_index[i] = nd_out_index[i];
  }

  nd_in_index[dim - 1] = in_size[dim - 1] - nd_out_index[dim - 1] - 1;
  nd_in_index[dim - 2] = in_size[dim - 2] - nd_out_index[dim - 2] - 1;

  int in_index = flatIndex(nd_in_index, in_size, dim);

  out[tid] = in[in_index];
}

extern "C" __global__ void dilate_tensor(float* in, float* out,
                            int* in_size, int* out_size,
                            int* dilate, int* in_index_buf,
                            int* out_index_buf, const int dim) {
  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  int* nd_in_index = in_index_buf + tid * dim;
  int* nd_out_index = out_index_buf + tid * dim;

  unflatIndex(nd_out_index, tid, out_size, dim);

  float out_value = 0;

  if (nd_out_index[dim - 1] % dilate[1] == 0 &&
      nd_out_index[dim - 2] % dilate[0] == 0) {
    for (int i = 0; i < dim; i++) {
      nd_in_index[i] = nd_out_index[i];
    }

    nd_in_index[dim - 1] /= dilate[1];
    nd_in_index[dim - 2] /= dilate[0];

    int in_index = flatIndex(nd_in_index, in_size, dim);
    out_value = in[in_index];
  }

  out[tid] = out_value;
}

extern "C" __global__ void softmax_exp(float* input, float* output, int n) {
  unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  if (tid >= (unsigned int)n) {
    return;
  }

  output[tid] = expf(input[tid]);
}

extern "C" __global__ void softmax_div(float* exp_input, float* out,
                          float* denominator, int num_element,
                          int batch_size) {
  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  if (tid > num_element) {
    return;
  }

  int num_element_per_image = num_element / batch_size;
  int batch = tid / num_element_per_image;
  out[tid] = exp_input[tid] / denominator[batch];
}

__device__ void sum_out_index_to_in_index(int* nd_out_index,
                               int* nd_in_index, int index, int axis,
                               int in_dim) {
  int axis_index_added = 0;
  for (int i = 0; i < in_dim; i++) {
    if (i == axis) {
      nd_in_index[i] = index;
      axis_index_added = 1;
    } else if (!axis_index_added) {
      nd_in_index[i] = nd_out_index[i];
    } else {
      nd_in_index[i] = nd_out_index[i - 1];
    }
  }
}

extern "C" __global__ void sum_one_axis(float* in, float* out,
                           int* in_size, int* out_size,
                           int in_dim, int axis, int* in_index_buf,
                           int* out_index_buf) {
  int global_id = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  int* nd_in_index = in_index_buf + global_id * in_dim;
  int* nd_out_index = out_index_buf + global_id * (in_dim - 1);

  unflatIndex(nd_out_index, global_id, out_size, in_dim - 1);

  float sum = 0.0f;
  for (int i = 0; i < in_size[axis]; i++) {
    sum_out_index_to_in_index(nd_out_index, nd_in_index, i, axis, in_dim);
    int in_flat_index = flatIndex(nd_in_index, in_size, in_dim);
    sum += in[in_flat_index];
  }

  out[global_id] = sum;
}

extern "C" __global__ void scaleAdd(float* out, float* in1,
                       float* in2, float alpha, float beta, int n) {
  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  if (tid > n) {
    return;
  }

  out[tid] = alpha * in1[tid] + beta * in2[tid];
}

extern "C" __global__ void mul(float* out, float* in1, float* in2, int n) {
  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  if (tid > n) {
    return;
  }

  out[tid] = in1[tid] * in2[tid];
}

extern "C" __global__ void rmsProp(float* params, float* gradients,
                      float* sHistory, float smoothFactor,
                      float learningRate, int n) {
  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  if (tid > n) {
    return;
  }
  sHistory[tid] = smoothFactor * sHistory[tid] +
                  (1 - smoothFactor) * gradients[tid] * gradients[tid];

  float sqrt_shistory = sqrtf(sHistory[tid]) + 1e-6f;
  float direction = gradients[tid] / sqrt_shistory;
  params[tid] -= learningRate * direction;
}

extern "C" __global__ void adam(float* params, float* gradients,
                   float* sHistory, float* vHistory,
                   float smoothFactor1, float smoothFactor2, float learningRate,
                   int n) {
  int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  if (tid > n) {
    return;
  }

  float vHistoryPart1 = smoothFactor1 * vHistory[tid];
  float vHistoryPart2 = (1 - smoothFactor1) * gradients[tid];
  vHistory[tid] = vHistoryPart1 + vHistoryPart2;
  sHistory[tid] = smoothFactor2 * sHistory[tid] +
                  (1 - smoothFactor2) * gradients[tid] * gradients[tid];

  float squareRoot = (sqrtf(sHistory[tid]) + 1e-8f);
  float direction = vHistory[tid] / squareRoot;
  params[tid] -= learningRate * direction;
}

extern "C" __global__ void reluForward(float* in, float* out, int count) {
  int index = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  if (index >= count) {
    return;
  }

  out[index] = in[index] > 0 ? in[index] : 0;
}

extern "C" __global__ void reluBackward(float* in, float* backin,
                           float* out, int count) {
  int index = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  if (index >= count) {
    return;
  }

  out[index] = in[index] > 0 ? backin[index] : 0;
}
