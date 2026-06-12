#include <hip/hip_runtime.h>

typedef int DATA_TYPE;

__global__ void pathfinder_kernel(DATA_TYPE *d_src, DATA_TYPE *d_dst,
                                  DATA_TYPE *d_wall, int width, int row) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (tid < width) {
        int offset = row * width;
        DATA_TYPE prev = d_src[tid];
        DATA_TYPE left = (tid > 0) ? d_src[tid - 1] : prev;
        DATA_TYPE right = (tid < width - 1) ? d_src[tid + 1] : prev;

        // Minimum of three neighbors from previous row
        DATA_TYPE min_val = prev;
        if (left < min_val) min_val = left;
        if (right < min_val) min_val = right;

        d_dst[tid] = d_wall[offset + tid] + min_val;
    }
}
