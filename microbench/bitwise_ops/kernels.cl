__kernel void microbench(__global int* v1, __global int* v2 ,__global int* output){

  uint tid = get_global_id(0);
  output[tid] = v1[tid] & v2[tid];
}
