__kernel void copyKernel(__global const float * d_in, __global float * d_out, const int N) {
    int global_id = get_global_id(0);

    if (global_id >= N) return;

    d_out[global_id] = d_in[global_id];

}
