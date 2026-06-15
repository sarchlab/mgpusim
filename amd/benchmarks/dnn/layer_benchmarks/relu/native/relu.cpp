/*
ReLU HIP kernel for gfx942
*/

#include <hip/hip_runtime.h>

extern "C" __global__
void ReLUForward(const int count, float* in, float* out) {
    int index = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    if (index < count)
        out[index] = in[index] > 0 ? in[index] : 0;
}

int main() {
    return 0;
}
