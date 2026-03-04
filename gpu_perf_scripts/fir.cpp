// fir.cpp — HIP benchmark for FIR (Finite Impulse Response) filter
// Kernel copied from amd/benchmarks/heteromark/fir/native/fir.cpp
// Problem size: length=8192, numTaps=16 per issue #153

#include "bench_common.h"

__global__ void FIR(
    float* output,
    float* coeff,
    float* input,
    float* history,
    unsigned int num_tap)
{
    unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    unsigned int num_data = hipGridDim_x * hipBlockDim_x;

    float sum = 0.0f;
    for (unsigned int i = 0; i < num_tap; i++) {
        if (tid >= i) {
            sum = sum + coeff[i] * input[tid - i];
        } else {
            sum = sum + coeff[i] * history[num_tap - (i - tid)];
        }
    }
    output[tid] = sum;
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    // LENGTH must be a multiple of WG_SIZE (256)
    int LENGTH   = parseIntParam(argc, argv, "--length", 8192);
    const int NUM_TAPS = 16;
    const int WG_SIZE  = 256;

    // Host allocations
    float* hInput   = (float*)malloc(LENGTH * sizeof(float));
    float* hOutput  = (float*)malloc(LENGTH * sizeof(float));
    float* hCoeff   = (float*)malloc(NUM_TAPS * sizeof(float));
    float* hHistory = (float*)malloc(NUM_TAPS * sizeof(float));

    // Initialize
    for (int i = 0; i < LENGTH; i++) {
        hInput[i] = (float)i;
    }
    for (int i = 0; i < NUM_TAPS; i++) {
        hCoeff[i] = (float)i;
        hHistory[i] = 0.0f;
    }

    // Device allocations
    float *dInput, *dOutput, *dCoeff, *dHistory;
    HIP_CHECK(hipMalloc(&dInput, LENGTH * sizeof(float)));
    HIP_CHECK(hipMalloc(&dOutput, LENGTH * sizeof(float)));
    HIP_CHECK(hipMalloc(&dCoeff, NUM_TAPS * sizeof(float)));
    HIP_CHECK(hipMalloc(&dHistory, NUM_TAPS * sizeof(float)));

    HIP_CHECK(hipMemcpy(dInput, hInput, LENGTH * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(dCoeff, hCoeff, NUM_TAPS * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(dHistory, hHistory, NUM_TAPS * sizeof(float), hipMemcpyHostToDevice));

    dim3 block(WG_SIZE, 1, 1);
    dim3 grid(LENGTH / WG_SIZE, 1, 1);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d_taps%d", LENGTH, NUM_TAPS);

    BenchResult r = runBenchmark("fir", problemSize, iterations, [&]() {
        FIR<<<grid, block>>>(dOutput, dCoeff, dInput, dHistory, NUM_TAPS);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(dInput));
    HIP_CHECK(hipFree(dOutput));
    HIP_CHECK(hipFree(dCoeff));
    HIP_CHECK(hipFree(dHistory));
    free(hInput);
    free(hOutput);
    free(hCoeff);
    free(hHistory);

    return 0;
}
