__kernel void pushData(__global float* src, __global float* dst,
                       const int num_element, const int num_thread) {
  const int tid = get_global_id(0);

  int iter = 0;

  while (true) {
    int id = iter * num_thread + tid;

    if (id >= num_element) {
      return;
    }

    dst[id] = src[id];
    iter++;
  }
}
