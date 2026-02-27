/*
 * HIP kernel for Simple Convolution (gfx942 / CDNA3)
 * Translated from SimpleConvolution_Kernels.cl
 *
 * Non-separable convolution where each output pixel is the weighted sum
 * of its neighbourhood pixels defined by a mask.
 */
#include "hip/hip_runtime.h"

extern "C" __global__ void simpleNonSeparableConvolution(
    unsigned int* input,
    float* mask,
    int* output,
    unsigned int inputWidth,
    unsigned int inputHeight,
    unsigned int maskWidth,
    unsigned int maskHeight,
    unsigned int nExWidth)
{
    unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    unsigned int width  = inputWidth;
    unsigned int height = inputHeight;

    unsigned int x = tid % width;
    unsigned int y = tid / width;

    if (x >= width || y >= height)
        return;

    /*
     * initializing weighted sum value
     */
    float sumFX = 0.0f;
    int m = 0, n = 0;

    // performing weighted sum within the mask boundaries
    for (unsigned int j = y; j < (y + maskHeight); ++j, m++)
    {
        n = 0;
        for (unsigned int i = x; i < (x + maskWidth); ++i, n++)
        {
            unsigned int maskIndex = m * maskWidth + n;
            unsigned int index     = j * nExWidth + i;

            sumFX += ((float)input[index] * mask[maskIndex]);
        }
    }

    sumFX += 0.5f;
    output[tid] = (int)sumFX;
}
