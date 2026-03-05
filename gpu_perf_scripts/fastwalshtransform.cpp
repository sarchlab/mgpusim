// fastwalshtransform.cpp — HIP benchmark for Fast Walsh Transform
// Kernel copied from amd/benchmarks/amdappsdk/fastwalshtransform/native/fastwalshtransform_hip.cpp
// Problem size: 1024 elements per issue #153

#include "bench_common.h"

__global__ void fastWalshTransform(
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

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    // LENGTH must be a power of 2 and >= 512
    unsigned int LENGTH = (unsigned int)parseIntParam(argc, argv, "--size", 1024);
    const unsigned int GLOBAL_THREADS = LENGTH / 2;
    const unsigned int LOCAL_THREADS  = 256;

    // Host allocation
    float* hInput = (float*)malloc(LENGTH * sizeof(float));

    // Initialize with random data
    srand(123);
    for (unsigned int i = 0; i < LENGTH; i++) {
        hInput[i] = (float)(rand() % 256) + ((float)rand() / (float)RAND_MAX);
    }

    // Keep a copy for re-upload each iteration
    float* hInputCopy = (float*)malloc(LENGTH * sizeof(float));
    memcpy(hInputCopy, hInput, LENGTH * sizeof(float));

    // Device allocation
    float* dInput;
    HIP_CHECK(hipMalloc(&dInput, LENGTH * sizeof(float)));

    dim3 block(LOCAL_THREADS, 1, 1);
    dim3 grid((GLOBAL_THREADS + LOCAL_THREADS - 1) / LOCAL_THREADS, 1, 1);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%u", LENGTH);

    BenchResult r = runBenchmark("fastwalshtransform", problemSize, iterations, [&]() {
        // Re-upload data each iteration since kernel modifies it in-place
        HIP_CHECK(hipMemcpy(dInput, hInputCopy, LENGTH * sizeof(float), hipMemcpyHostToDevice));

        // Run all passes: step = 1, 2, 4, ..., LENGTH/2
        for (unsigned int step = 1; step < LENGTH; step <<= 1) {
            fastWalshTransform<<<grid, block>>>(dInput, (int)step);
        }
        HIP_CHECK(hipDeviceSynchronize());
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(dInput));
    free(hInput);
    free(hInputCopy);

    return 0;
}
