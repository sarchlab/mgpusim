__kernel void cross_entropy_derivative(__global float *output,
                                       const __global float *input,
                                       const __global int *label,
                                       int batch_size, int num_per_image) {
  int gid = get_global_id(0);

  int batch = gid / num_per_image;
  int elem = gid % num_per_image;

  if (label[batch] == elem) {
    output[gid] = -1 / input[gid];
  } else {
    output[gid] = 0;
  }
}

__kernel void softmax_cross_entropy_derivative(__global float *output,
                                               const __global float *input,
                                               const __global int *label,
                                               int batch_size,
                                               int num_per_image) {
  int gid = get_global_id(0);

  int batch = gid / num_per_image;
  int elem = gid % num_per_image;

  if (label[batch] == elem) {
    output[gid] = input[gid] - 1;
  } else {
    output[gid] = input[gid];
  }
}