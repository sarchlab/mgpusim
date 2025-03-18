__kernel void reduceData(__global float* buf, __global float* store,
                         const int num_element, const int num_thread,
                         const int gpu_n, const int lastReduce) {
  const int tid = get_global_id(0);

  int iter = 0;

  if (lastReduce == 0) {
    while (true) {
      int id = iter * num_thread + tid;

      if (id >= num_element) {
        return;
      }

      store[id] += buf[id];
      iter++;
    }
  } else {
    while (true) {
      int id = iter * num_thread + tid;

      if (id >= num_element) {
        return;
      }

      store[id] += buf[id];
      store[id] /= gpu_n;
      iter++;
    }
  }
}