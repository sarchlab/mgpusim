/*
 * HIP kernel for Fast Walsh Transform (gfx942 / CDNA3)
 * Translated from FastWalshTransform_Kernels.cl
 */
#include "hip/hip_runtime.h"

extern "C" __global__ void fastWalshTransform(
    float* tArray,
    const int step)
{
    unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    const unsigned int group = tid % step;
    const unsigned int pair  = 2 * step * (tid / step) + group;

    const unsigned int match = pair + step;

    float T1 = tArray[pair];
    float T2 = tArray[match];

    tArray[pair]  = T1 + T2;
    tArray[match] = T1 - T2;
}
