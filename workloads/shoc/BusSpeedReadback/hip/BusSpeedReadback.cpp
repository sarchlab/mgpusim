// BusSpeedReadback.cpp -- SHOC BusSpeedReadback benchmark (HIP)
// Measures device-to-host PCIe transfer bandwidth.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    // transfer_size_kb: transfer size in kilobytes
    int transfer_size_kb = parseIntParam(argc, argv, "--transfer_size_kb", 1024);
    int iters            = parseIterations(argc, argv);

    size_t bytes = (size_t)transfer_size_kb * 1024;

    // Allocate pinned host memory
    float* h_buf = nullptr;
    HIP_CHECK(hipHostMalloc(&h_buf, bytes));

    // Allocate device memory and fill it
    float* d_buf = nullptr;
    HIP_CHECK(hipMalloc(&d_buf, bytes));

    // Fill device buffer with data (upload first)
    size_t num_floats = bytes / sizeof(float);
    for (size_t i = 0; i < num_floats; ++i) {
        h_buf[i] = (float)(i % 1000) * 0.001f;
    }
    HIP_CHECK(hipMemcpy(d_buf, h_buf, bytes, hipMemcpyHostToDevice));

    // Benchmark: device-to-host transfer
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%dKB", transfer_size_kb);

    printCSVHeader();

    BenchResult r = runBenchmark("BusSpeedReadback", size_str, iters, [&]() {
        HIP_CHECK(hipMemcpy(h_buf, d_buf, bytes, hipMemcpyDeviceToHost));
    });

    printCSVRow(r);

    // Report bandwidth
    double bandwidth_gb = (double)bytes / (r.avg_ms * 1e-3) / 1e9;
    fprintf(stderr, "Transfer size: %d KB, Bandwidth: %.2f GB/s\n",
            transfer_size_kb, bandwidth_gb);

    // Cleanup
    HIP_CHECK(hipFree(d_buf));
    HIP_CHECK(hipHostFree(h_buf));

    return 0;
}
