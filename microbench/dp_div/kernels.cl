__kernel void microbench(__global double* v1, __global double* v2, __global double* output){

  uint tid = get_global_id(0);

 for(int i = 0; i < 10000; i++)
  output[tid] = v1[tid] / v2[tid];
}
