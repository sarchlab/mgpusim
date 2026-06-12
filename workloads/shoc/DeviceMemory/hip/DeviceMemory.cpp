// DeviceMemory.cpp -- SHOC DeviceMemory benchmark (HIP)
// Measures on-device global memory read and write bandwidth.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Kernels
// ---------------------------------------------------------------------------

// Read kernel: each thread reads N_PER_THREAD elements, accumulates them
__global__ void read_kernel(const float* __restrict__ input,
                            float* __restrict__ output,
                            int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    int totalThreads = gridDim.x * blockDim.x;

    float sum = 0.0f;
    for (int i = gid; i < N; i += totalThreads) {
        sum += input[i];
    }
    // Write single result to prevent optimization
    if (gid < N) {
        output[gid] = sum;
    }
}

// Write kernel: each thread writes elements in a strided pattern
__global__ void write_kernel(float* __restrict__ output,
                             int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    int totalThreads = gridDim.x * blockDim.x;

    for (int i = gid; i < N; i += totalThreads) {
        output[i] = (float)i;
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int array_size_mb = parseIntParam(argc, argv, "--array_size_mb", 64);
    int block_size    = parseIntParam(argc, argv, "--block_size", 256);
    int iters         = parseIterations(argc, argv);

    size_t N     = (size_t)array_size_mb * 1024 * 1024 / sizeof(float);
    size_t bytes = N * sizeof(float);

    // Device allocations
    float *d_input, *d_output;
    HIP_CHECK(hipMalloc(&d_input, bytes));
    HIP_CHECK(hipMalloc(&d_output, bytes));

    // Initialize device input
    float* h_init = (float*)malloc(bytes);
    for (size_t i = 0; i < N; ++i) {
        h_init[i] = (float)(i % 1000) * 0.001f;
    }
    HIP_CHECK(hipMemcpy(d_input, h_init, bytes, hipMemcpyHostToDevice));
    free(h_init);

    // Use enough blocks to saturate the GPU
    int num_blocks = (int)((N + block_size - 1) / block_size);
    // Cap at a reasonable maximum to avoid excessive grid size
    if (num_blocks > 65535) num_blocks = 65535;

    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%dMB", array_size_mb);

    printCSVHeader();

    // Benchmark read bandwidth
    BenchResult r_read = runBenchmark("DeviceMemory_Read", size_str, iters, [&]() {
        hipLaunchKernelGGL(read_kernel, dim3(num_blocks), dim3(block_size),
                           0, 0, d_input, d_output, (int)N);
    });
    printCSVRow(r_read);

    double read_bw_gb = (double)bytes / (r_read.avg_ms * 1e-3) / 1e9;
    fprintf(stderr, "Read bandwidth: %.2f GB/s (%d MB)\n", read_bw_gb, array_size_mb);

    // Benchmark write bandwidth
    BenchResult r_write = runBenchmark("DeviceMemory_Write", size_str, iters, [&]() {
        hipLaunchKernelGGL(write_kernel, dim3(num_blocks), dim3(block_size),
                           0, 0, d_output, (int)N);
    });
    printCSVRow(r_write);

    double write_bw_gb = (double)bytes / (r_write.avg_ms * 1e-3) / 1e9;
    fprintf(stderr, "Write bandwidth: %.2f GB/s (%d MB)\n", write_bw_gb, array_size_mb);

    // Cleanup
    HIP_CHECK(hipFree(d_input));
    HIP_CHECK(hipFree(d_output));

    return 0;
}
