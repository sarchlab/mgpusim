// memcopy.cpp — HIP benchmark for device memory copy (hipMemcpy)
// No native kernel — measures hipMemcpy H2D and D2H throughput
// Problem size: 4MB per issue #153

#include "bench_common.h"

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int sizeMB = parseIntParam(argc, argv, "--size", 4);

    const size_t SIZE = (size_t)sizeMB * 1024 * 1024;
    const size_t NUM  = SIZE / sizeof(float);

    // Host allocation
    float* hostSrc = (float*)malloc(SIZE);
    float* hostDst = (float*)malloc(SIZE);
    for (size_t i = 0; i < NUM; i++) {
        hostSrc[i] = (float)i;
    }

    // Device allocation
    float* deviceBuf;
    HIP_CHECK(hipMalloc(&deviceBuf, SIZE));

    char problemSize[32];
    snprintf(problemSize, sizeof(problemSize), "%dMB", sizeMB);

    // Benchmark Host-to-Device
    {
        BenchResult r = runBenchmark("memcopy_h2d", problemSize, iterations, [&]() {
            HIP_CHECK(hipMemcpy(deviceBuf, hostSrc, SIZE, hipMemcpyHostToDevice));
        });
        printCSVHeader();
        printCSVRow(r);
    }

    // Benchmark Device-to-Host
    {
        BenchResult r = runBenchmark("memcopy_d2h", problemSize, iterations, [&]() {
            HIP_CHECK(hipMemcpy(hostDst, deviceBuf, SIZE, hipMemcpyDeviceToHost));
        });
        printCSVRow(r);
    }

    // Benchmark Device-to-Device
    {
        float* deviceBuf2;
        HIP_CHECK(hipMalloc(&deviceBuf2, SIZE));

        BenchResult r = runBenchmark("memcopy_d2d", problemSize, iterations, [&]() {
            HIP_CHECK(hipMemcpy(deviceBuf2, deviceBuf, SIZE, hipMemcpyDeviceToDevice));
        });
        printCSVRow(r);

        HIP_CHECK(hipFree(deviceBuf2));
    }

    // Cleanup
    HIP_CHECK(hipFree(deviceBuf));
    free(hostSrc);
    free(hostDst);

    return 0;
}
