__kernel void microbench(__global float* in, unsigned repeat) {
  unsigned a;

  for (unsigned i = 0; i < repeat; i++) {
      a = in[0];
  }
}
