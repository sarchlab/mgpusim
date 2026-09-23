/*
 * HIP kernel for Floyd-Warshall (gfx942 / CDNA3)
 * Translated from FloydWarshall_Kernels.cl
 *
 * Multi-pass shortest path algorithm.
 * Each pass introduces node k and updates the distance matrix.
 */
#include "hip/hip_runtime.h"

extern "C" __global__ void floydWarshallPass(
    unsigned int* pathDistanceBuffer,
    unsigned int* pathBuffer,
    const unsigned int numNodes,
    const unsigned int pass)
{
    int xValue = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    int yValue = hipBlockDim_y * hipBlockIdx_y + hipThreadIdx_y;

    int k = pass;
    int oldWeight = pathDistanceBuffer[yValue * numNodes + xValue];
    int tempWeight = (pathDistanceBuffer[yValue * numNodes + k] +
                      pathDistanceBuffer[k * numNodes + xValue]);

    if (tempWeight < oldWeight)
    {
        pathDistanceBuffer[yValue * numNodes + xValue] = tempWeight;
        pathBuffer[yValue * numNodes + xValue] = k;
    }
}
