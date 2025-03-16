__kernel void repeat(__global float *output, __global float *input,
                     const uint input_length, const uint output_length) {
  uint tid = get_global_id(0);

  if (tid > output_length) {
    return;
  }

  uint input_index = tid % input_length;

  output[tid] = input[input_index];
}