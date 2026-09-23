/**
 * bitonicsort.cpp: HIP implementation of BitonicSort kernel
 * Translated from kernels.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

extern "C" __global__ void BitonicSort(unsigned int* array, const unsigned int stage,
                          const unsigned int passOfStage, const unsigned int direction) {
  unsigned int sortIncreasing = direction;
  unsigned int threadId = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  unsigned int pairDistance = 1 << (stage - passOfStage);
  unsigned int blockWidth = 2 * pairDistance;

  unsigned int leftId =
      (threadId % pairDistance) + (threadId / pairDistance) * blockWidth;

  unsigned int rightId = leftId + pairDistance;

  unsigned int leftElement = array[leftId];
  unsigned int rightElement = array[rightId];

  unsigned int sameDirectionBlockWidth = 1 << stage;

  if ((threadId / sameDirectionBlockWidth) % 2 == 1)
    sortIncreasing = 1 - sortIncreasing;

  unsigned int greater;
  unsigned int lesser;
  if (leftElement > rightElement) {
    greater = leftElement;
    lesser = rightElement;
  } else {
    greater = rightElement;
    lesser = leftElement;
  }

  if (sortIncreasing) {
    array[leftId] = lesser;
    array[rightId] = greater;
  } else {
    array[leftId] = greater;
    array[rightId] = lesser;
  }
}
