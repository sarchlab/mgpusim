__kernel void microbench(__global int* output){

  output[get_global_id(0)] = 3;

}
