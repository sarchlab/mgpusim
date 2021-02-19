int numElement(__global int* size, int dim) {
  int s = 1;

  for (int i = 0; i < dim; i++) {
    s *= size[i];
  }

  return s;
}

void unflatIndex(__global int* nd_index, int flat_index, __global int* size,
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

int flatIndex(__global int* nd_index, __global int* size, int dim) {
  int out = 0;
  int total_size = 1;

  for (int i = 0; i < dim; i++) {
    out += nd_index[dim - i - 1] * total_size;
    total_size *= size[dim - i - 1];
  }

  return out;
}

__kernel void transpose_tensor(__global float* in, __global float* out,
                               __global int* in_size, __global int* out_size,
                               __global int* order, __global int* in_index_buf,
                               __global int* out_index_buf, const int dim) {
  int tid = get_global_id(0);

  __global int* nd_in_index = in_index_buf + tid * dim;
  __global int* nd_out_index = out_index_buf + tid * dim;

  unflatIndex(nd_out_index, tid, out_size, dim);

  for (int i = 0; i < dim; i++) {
    nd_in_index[order[i]] = nd_out_index[i];
  }

  int input_index_flat = flatIndex(nd_in_index, in_size, dim);

  out[tid] = in[input_index_flat];
}

__kernel void rotate_tensor(__global float* in, __global float* out,
                            __global int* in_size, __global int* out_size,
                            __global int* in_index_buf,
                            __global int* out_index_buf, const int dim) {
  int tid = get_global_id(0);

  __global int* nd_in_index = in_index_buf + tid * dim;
  __global int* nd_out_index = out_index_buf + tid * dim;

  unflatIndex(nd_out_index, tid, out_size, dim);

  for (int i = 0; i < dim; i++) {
    nd_in_index[i] = nd_out_index[i];
  }

  nd_in_index[dim - 1] = in_size[dim - 1] - nd_out_index[dim - 1] - 1;
  nd_in_index[dim - 2] = in_size[dim - 2] - nd_out_index[dim - 2] - 1;

  int in_index = flatIndex(nd_in_index, in_size, dim);

  out[tid] = in[in_index];
}

__kernel void dilate_tensor(__global float* in, __global float* out,
                            __global int* in_size, __global int* out_size,
                            __global int* dilate, __global int* in_index_buf,
                            __global int* out_index_buf, const int dim) {
  int tid = get_global_id(0);

  __global int* nd_in_index = in_index_buf + tid * dim;
  __global int* nd_out_index = out_index_buf + tid * dim;

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

__kernel void softmax_exp(__global float* input, __global float* output,
                          int n) {
  uint tid = get_global_id(0);

  if (tid >= n) {
    return;
  }

  output[tid] = exp(input[tid]);
}

__kernel void softmax_div(__global float* exp_input, __global float* out,
                          __global float* denominator, int num_element,
                          int batch_size) {
  int tid = get_global_id(0);

  if (tid > num_element) {
    return;
  }

  int num_element_per_image = num_element / batch_size;
  int batch = tid / num_element_per_image;
  out[tid] = exp_input[tid] / denominator[batch];
}

void sum_out_index_to_in_index(__global int* nd_out_index,
                               __global int* nd_in_index, int index, int axis,
                               int in_dim) {
  int axis_index_added = false;
  for (int i = 0; i < in_dim; i++) {
    if (i == axis) {
      nd_in_index[i] = index;
      axis_index_added = true;
    } else if (!axis_index_added) {
      nd_in_index[i] = nd_out_index[i];
    } else {
      nd_in_index[i] = nd_out_index[i - 1];
    }
  }
}

__kernel void sum_one_axis(__global float* in, __global float* out,
                           __global int* in_size, __global int* out_size,
                           int in_dim, int axis, __global int* in_index_buf,
                           __global int* out_index_buf) {
  int global_id = get_global_id(0);

  __global int* nd_in_index = in_index_buf + global_id * in_dim;
  __global int* nd_out_index = out_index_buf + global_id * (in_dim - 1);

  unflatIndex(nd_out_index, global_id, out_size, in_dim - 1);

  float sum = 0.0;
  for (int i = 0; i < in_size[axis]; i++) {
    sum_out_index_to_in_index(nd_out_index, nd_in_index, i, axis, in_dim);
    int in_flat_index = flatIndex(nd_in_index, in_size, in_dim);
    sum += in[in_flat_index];
  }

  out[global_id] = sum;
}

__kernel void scaleAdd(__global float* out, __global float* in1,
                       __global float* in2, float alpha, float beta, int n) {
  int tid = get_global_id(0);
  if (tid > n) {
    return;
  }

  out[tid] = alpha * in1[tid] + beta * in2[tid];
}

__kernel void mul(__global float* out, __global float* in1, __global float* in2,
                  int n) {
  int tid = get_global_id(0);
  if (tid > n) {
    return;
  }

  out[tid] = in1[tid] * in2[tid];
}

__kernel void rmsProp(__global float* params, __global float* gradients,
                      __global float* sHistory, float smoothFactor,
                      float learningRate, int n) {
  int tid = get_global_id(0);
  if (tid > n) {
    return;
  }
  sHistory[tid] = smoothFactor * sHistory[tid] +
                  (1 - smoothFactor) * gradients[tid] * gradients[tid];

  float sqrt_shistory = sqrt(sHistory[tid]) + 1e-6;
  float direction = gradients[tid] / sqrt_shistory;
  params[tid] -= learningRate * direction;
}

__kernel void adam(__global float* params, __global float* gradients,
                   __global float* sHistory, __global float* vHistory,
                   float smoothFactor1, float smoothFactor2, float learningRate,
                   int n) {
  int tid = get_global_id(0);
  if (tid > n) {
    return;
  }

  float vHistoryPart1 = smoothFactor1 * vHistory[tid];
  float vHistoryPart2 = (1 - smoothFactor1) * gradients[tid];
  vHistory[tid] = vHistoryPart1 + vHistoryPart2;
  sHistory[tid] = smoothFactor2 * sHistory[tid] +
                  (1 - smoothFactor2) * gradients[tid] * gradients[tid];

  float squareRoot = (sqrt(sHistory[tid]) + 1e-8);
  float direction = vHistory[tid] / squareRoot;
  params[tid] -= learningRate * direction;
}

__kernel void reluForward(__global float* in, __global float* out, int count) {
  int index = get_global_id(0);

  if (index >= count) {
    return;
  }

  out[index] = in[index] > 0 ? in[index] : 0;
}

__kernel void reluBackward(__global float* in, __global float* backin,
                           __global float* out, int count) {
  int index = get_global_id(0);
  if (index >= count) {
    return;
  }

  out[index] = in[index] > 0 ? backin[index] : 0;
}