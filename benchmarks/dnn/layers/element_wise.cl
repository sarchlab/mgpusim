__kernel void add(__global float *out, __global float *in1, __global float *in2,
                  int n) {
  int tid = get_global_id(0);
  if (tid > n) {
    return;
  }

  out[tid] = in1[tid] + in2[tid];
}