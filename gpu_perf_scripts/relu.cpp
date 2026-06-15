// relu.cpp — HIP benchmark for ReLU forward activation
// Kernel copied from amd/benchmarks/dnn/layer_benchmarks/relu/native/relu.cpp
// Problem size: 65536 elements

#include "bench_common.h"

__global__ void ReLUForward(const int count, float* in, float* out) {
    int index = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    if (index < count)
        out[index] = in[index] > 0 ? in[index] : 0;
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int COUNT = parseIntParam(argc, argv, "--size", 65536);

    // Host allocation
    float* h_in = (float*)malloc(COUNT * sizeof(float));

    srand(42);
    for (int i = 0; i < COUNT; i++)
        h_in[i] = (float)(rand() % 2000 - 1000) / 100.0f;  // range [-10, 10]

    // Device allocations
    float *d_in, *d_out;
    HIP_CHECK(hipMalloc(&d_in, COUNT * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_out, COUNT * sizeof(float)));

    HIP_CHECK(hipMemcpy(d_in, h_in, COUNT * sizeof(float), hipMemcpyHostToDevice));

    const int THREADS = 256;
    dim3 block(THREADS);
    dim3 grid((COUNT + THREADS - 1) / THREADS);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", COUNT);

    BenchResult r = runBenchmark("relu", problemSize, iterations, [&]() {
        ReLUForward<<<grid, block>>>(COUNT, d_in, d_out);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_in));
    HIP_CHECK(hipFree(d_out));
    free(h_in);

    return 0;
}
