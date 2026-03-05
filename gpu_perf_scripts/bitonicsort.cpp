// bitonicsort.cpp — HIP benchmark for Bitonic Sort
// Kernel copied from amd/benchmarks/amdappsdk/bitonicsort/native/bitonicsort.cpp
// Problem size: 4096 elements per issue #153

#include "bench_common.h"

__global__ void BitonicSort(unsigned int* array,
                            const unsigned int stage,
                            const unsigned int passOfStage,
                            const unsigned int direction) {
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

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    // LENGTH must be a power of 2
    int LENGTH = parseIntParam(argc, argv, "--size", 4096);
    const int WG_SIZE = 64;
    const int NUM_THREADS = LENGTH / 2;  // each thread handles one compare-swap
    const unsigned int DIRECTION = 1;     // ascending

    // Host allocation
    unsigned int* hInput = (unsigned int*)malloc(LENGTH * sizeof(unsigned int));

    // Initialize with random data
    srand(42);
    for (int i = 0; i < LENGTH; i++) {
        hInput[i] = (unsigned int)rand();
    }

    // Keep a copy for re-upload each iteration
    unsigned int* hInputCopy = (unsigned int*)malloc(LENGTH * sizeof(unsigned int));
    memcpy(hInputCopy, hInput, LENGTH * sizeof(unsigned int));

    // Device allocation
    unsigned int* dArray;
    HIP_CHECK(hipMalloc(&dArray, LENGTH * sizeof(unsigned int)));

    // Count stages
    int numStages = 0;
    for (int temp = LENGTH; temp > 1; temp >>= 1) {
        numStages++;
    }

    dim3 block(WG_SIZE, 1, 1);
    dim3 grid((NUM_THREADS + WG_SIZE - 1) / WG_SIZE, 1, 1);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", LENGTH);

    BenchResult r = runBenchmark("bitonicsort", problemSize, iterations, [&]() {
        // Re-upload data each iteration since sort modifies in-place
        HIP_CHECK(hipMemcpy(dArray, hInputCopy, LENGTH * sizeof(unsigned int), hipMemcpyHostToDevice));

        for (int stage = 0; stage < numStages; stage++) {
            for (int passOfStage = 0; passOfStage < stage + 1; passOfStage++) {
                BitonicSort<<<grid, block>>>(dArray, stage, passOfStage, DIRECTION);
            }
        }
        HIP_CHECK(hipDeviceSynchronize());
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(dArray));
    free(hInput);
    free(hInputCopy);

    return 0;
}
