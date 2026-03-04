// vectoradd.cpp — HIP benchmark for vector addition
// Kernel copied from amd/benchmarks/amdappsdk/vectoradd/native/vectoradd.cpp
// Problem size: 4096 elements (width=4096, height=1) per issue #153

#include "bench_common.h"

__global__ void vectoradd_float(float* __restrict__ a,
                                const float* __restrict__ b,
                                const float* __restrict__ c,
                                int width, int height) {
    int x = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    int y = hipBlockDim_y * hipBlockIdx_y + hipThreadIdx_y;

    int i = y * width + x;
    if (i < (width * height)) {
        a[i] = b[i] + c[i];
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int WIDTH  = parseIntParam(argc, argv, "--size", 4096);
    const int HEIGHT = 1;
    const int NUM    = WIDTH * HEIGHT;

    // Host allocations
    float* hostB = (float*)malloc(NUM * sizeof(float));
    float* hostC = (float*)malloc(NUM * sizeof(float));

    for (int i = 0; i < NUM; i++) {
        hostB[i] = (float)i;
        hostC[i] = (float)i * 100.0f;
    }

    // Device allocations
    float *deviceA, *deviceB, *deviceC;
    HIP_CHECK(hipMalloc(&deviceA, NUM * sizeof(float)));
    HIP_CHECK(hipMalloc(&deviceB, NUM * sizeof(float)));
    HIP_CHECK(hipMalloc(&deviceC, NUM * sizeof(float)));

    HIP_CHECK(hipMemcpy(deviceB, hostB, NUM * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(deviceC, hostC, NUM * sizeof(float), hipMemcpyHostToDevice));

    const int THREADS = 256;
    dim3 block(THREADS, 1, 1);
    dim3 grid((WIDTH + THREADS - 1) / THREADS, HEIGHT, 1);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", NUM);

    BenchResult r = runBenchmark("vectoradd", problemSize, iterations, [&]() {
        vectoradd_float<<<grid, block>>>(deviceA, deviceB, deviceC, WIDTH, HEIGHT);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(deviceA));
    HIP_CHECK(hipFree(deviceB));
    HIP_CHECK(hipFree(deviceC));
    free(hostB);
    free(hostC);

    return 0;
}
