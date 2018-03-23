__kernel void microbench(__global float* v1, __global float* v2, __global float* output){

  uint tid = get_global_id(0);

 for(int i = 0; i < 1000; i++)
  output[tid] = v1[tid] + v2[tid];
}
