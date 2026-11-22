// Device source from GPU Benchmarks v1.1 (a8ca84f8), mixbench.hip.
#include <hip/hip_runtime.h>

extern "C" __global__ void mixbench_kernel(float *data, int num_elements, int num_fmas)
{
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid >= num_elements) return;

    float val = data[tid];

    /* Multiplier and addend chosen so the compiler cannot constant-fold the
       loop.  Values are close to 1 and 0 to avoid overflow/underflow. */
    const float mul = 1.0000001f;
    const float add = 0.0000001f;

    /* Dependency chain: each FMA uses the result of the previous one,
       preventing the compiler from eliminating the computation. */
    for (int i = 0; i < num_fmas; ++i) {
        val = fmaf(val, mul, add);
    }

    data[tid] = val;
}
