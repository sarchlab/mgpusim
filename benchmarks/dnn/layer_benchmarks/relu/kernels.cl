// ReLU
__kernel void ReLUForward(const int count, __global float* in, __global float* out) {
  int index = get_global_id(0);
  if(index < count)
    out[index] = in[index] > 0? in[index]:0;
}