// simpleconvolution.cpp — HIP benchmark for simple non-separable convolution
// Kernel copied from amd/benchmarks/amdappsdk/simpleconvolution/native/simpleconvolution.cpp
// Problem size: 512x512 with 3x3 mask per issue #153

#include "bench_common.h"

__global__ void simpleNonSeparableConvolution(
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
    for (unsigned int j = y; j < (y + maskHeight); ++j, m++) {
        n = 0;
        for (unsigned int i = x; i < (x + maskWidth); ++i, n++) {
            unsigned int maskIndex = m * maskWidth + n;
            unsigned int index     = j * nExWidth + i;

            sumFX += ((float)input[index] * mask[maskIndex]);
        }
    }

    sumFX += 0.5f;
    output[tid] = (int)sumFX;
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    unsigned int WIDTH     = (unsigned int)parseIntParam(argc, argv, "--size", 512);
    unsigned int HEIGHT    = WIDTH;
    unsigned int MASK_SIZE = (unsigned int)parseIntParam(argc, argv, "--mask", 3);
    const unsigned int PAD_W     = MASK_SIZE - 1;  // 2
    const unsigned int PAD_H     = MASK_SIZE - 1;  // 2
    const unsigned int NE_WIDTH  = WIDTH + PAD_W;   // extended width
    const unsigned int WG_SIZE   = 64;

    const unsigned int numOutputData = WIDTH * HEIGHT;
    const unsigned int numInputData  = NE_WIDTH * (HEIGHT + PAD_H);

    // Host allocations
    unsigned int* hInput = (unsigned int*)malloc(numInputData * sizeof(unsigned int));
    int*          hOutput = (int*)malloc(numOutputData * sizeof(int));
    float*        hMask   = (float*)malloc(MASK_SIZE * MASK_SIZE * sizeof(float));

    // Initialize
    for (unsigned int i = 0; i < numInputData; i++) {
        hInput[i] = 1;
    }
    for (unsigned int i = 0; i < MASK_SIZE * MASK_SIZE; i++) {
        hMask[i] = 1.0f;
    }

    // Device allocations
    unsigned int* dInput;
    int*          dOutput;
    float*        dMask;
    HIP_CHECK(hipMalloc(&dInput, numInputData * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&dOutput, numOutputData * sizeof(int)));
    HIP_CHECK(hipMalloc(&dMask, MASK_SIZE * MASK_SIZE * sizeof(float)));

    HIP_CHECK(hipMemcpy(dInput, hInput, numInputData * sizeof(unsigned int), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(dMask, hMask, MASK_SIZE * MASK_SIZE * sizeof(float), hipMemcpyHostToDevice));

    // Grid: total threads = numOutputData (one thread per output pixel)
    unsigned int totalThreads = numOutputData;
    dim3 block(WG_SIZE, 1, 1);
    dim3 grid((totalThreads + WG_SIZE - 1) / WG_SIZE, 1, 1);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%ux%u_mask%u",
             WIDTH, HEIGHT, MASK_SIZE);

    BenchResult r = runBenchmark("simpleconvolution", problemSize, iterations, [&]() {
        simpleNonSeparableConvolution<<<grid, block>>>(
            dInput, dMask, dOutput,
            WIDTH, HEIGHT,
            MASK_SIZE, MASK_SIZE,
            NE_WIDTH);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(dInput));
    HIP_CHECK(hipFree(dOutput));
    HIP_CHECK(hipFree(dMask));
    free(hInput);
    free(hOutput);
    free(hMask);

    return 0;
}
