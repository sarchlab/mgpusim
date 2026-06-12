// MaxFlops.cu -- SHOC MaxFlops benchmark (CUDA)
// Measures peak single-precision FLOPS using FMA chains.

#include "bench_common_cuda.h"

// ---------------------------------------------------------------------------
// Kernel
// ---------------------------------------------------------------------------
__global__ void maxflops_kernel(float* __restrict__ output,
                                int ops_per_thread) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;

    float a = 1.0f;
    float b = threadIdx.x * 0.001f;

    for (int i = 0; i < ops_per_thread; ++i) {
        a = a * b + b;  // FMA: 2 FLOPS
    }

    // Write result to prevent compiler optimization
    output[gid] = a;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int num_threads    = parseIntParam(argc, argv, "--num_threads", 1048576);
    int block_size     = parseIntParam(argc, argv, "--block_size", 256);
    int ops_per_thread = parseIntParam(argc, argv, "--ops_per_thread", 512);
    int iters          = parseIterations(argc, argv);

    // Ensure num_threads is a multiple of block_size
    int grid_size = (num_threads + block_size - 1) / block_size;
    int actual_threads = grid_size * block_size;

    size_t bytes = (size_t)actual_threads * sizeof(float);

    // Device allocation
    float* d_output = nullptr;
    CUDA_CHECK(cudaMalloc(&d_output, bytes));

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", num_threads);

    printCSVHeader();

    BenchResult r = runBenchmark("MaxFlops", size_str, iters, [&]() {
        maxflops_kernel<<<grid_size, block_size>>>(d_output, ops_per_thread);
    });

    printCSVRow(r);

    // Report GFLOPS: each FMA = 2 FLOPS
    double total_flops = (double)actual_threads * ops_per_thread * 2.0;
    double gflops = total_flops / (r.avg_ms * 1e-3) / 1e9;
    fprintf(stderr, "Threads: %d, OpsPerThread: %d, GFLOPS: %.2f\n",
            actual_threads, ops_per_thread, gflops);

    // Cleanup
    CUDA_CHECK(cudaFree(d_output));

    return 0;
}
