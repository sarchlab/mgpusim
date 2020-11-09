__kernel void dilate_tensor(__global float* in, __global float* out,
                            __global int* in_size, __global int* out_size,
                            __global int* dilate, __global int* in_index_buf,
                            __global int* out_index_buf, const int dim) {
  int tid = get_global_id(0);

  __global int* l_in_index_buf = in_index_buf + tid * dim;
  __global int* l_out_index_buf = out_index_buf + tid * dim;

  int size_left = tid;
  int accumulated_size = 1;
  for (int i = 0; i < dim; i++) {
    accumulated_size *= out_size[i];
  }

  for (int i = 0; i < dim; i++) {
    accumulated_size /= out_size[i];
    int index = size_left / accumulated_size;
    l_out_index_buf[i] = index;
    size_left -= index * accumulated_size;
  }

  float output = 0;
  if (l_out_index_buf[dim - 1] % dilate[1] == 0 &&
      l_out_index_buf[dim - 2] % dilate[0] == 0) {
    for (int i = 0; i < dim; i++) {
      l_in_index_buf[i] = l_out_index_buf[i];

      if (i == dim - 2) {
        l_in_index_buf[i] /= dilate[0];
      }

      if (i == dim - 1) {
        l_in_index_buf[i] /= dilate[1];
      }
    }

    accumulated_size = 1;
    int in_index = 0;
    for (int i = 0; i < dim; i++) {
      in_index += l_in_index_buf[dim - i - 1] * accumulated_size;
      accumulated_size *= in_size[dim - i - 1];
    }

    output = in[in_index];
  }

  out[tid] = output;
}