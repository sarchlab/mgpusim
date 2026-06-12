// occupancy_sweep.cpp -- HIP benchmark for occupancy vs performance
// A kernel that uses configurable amounts of shared memory and does useful
// FMA work. By varying block_size and shared_mem_bytes, occupancy changes,
// letting users measure occupancy-vs-performance tradeoffs.
// Reports GFLOPS.

#include "bench_common_hip.h"

// FMA kernel with configurable shared memory allocation
// Each thread processes one element, performing num_iterations FMA ops
__global__ void occupancy_fma_kernel(float *d_data, int num_elements,
                                      int num_iterations) {
    // Dynamically allocated shared memory -- occupancy limiter
    extern __shared__ float smem[];

    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    // Initialize shared memory (if any) to prevent it from being optimized away
    if (hipThreadIdx_x == 0 && hipBlockDim_x > 0) {
        smem[0] = 1.0f;
    }
    __syncthreads();

    if (tid < num_elements) {
        float val = d_data[tid];
        float a = 1.00001f;
        float b = 0.99999f;

        // FMA-heavy inner loop: 4 FMAs per iteration = 8 FLOPS per iteration
        for (int i = 0; i < num_iterations; i++) {
            val = val * a + b;   // FMA: 2 FLOPS
            val = val * b + a;   // FMA: 2 FLOPS
            val = val * a + b;   // FMA: 2 FLOPS
            val = val * b + a;   // FMA: 2 FLOPS
        }

        // Use shared memory value to prevent DCE of smem allocation
        val += smem[0] * 0.0f;

        d_data[tid] = val;
    }
}

int main(int argc, char **argv) {
    int iterations       = parseIterations(argc, argv);
    int num_elements     = parseIntParam(argc, argv, "--num_elements", 1048576);
    int block_size       = parseIntParam(argc, argv, "--block_size", 256);
    int shared_mem_bytes = parseIntParam(argc, argv, "--shared_mem_bytes", 4096);
    int num_iterations   = parseIntParam(argc, argv, "--num_iterations", 1024);

    // Ensure at least 4 bytes of shared memory for the smem[0] access
    if (shared_mem_bytes < 4) shared_mem_bytes = 4;

    int grid_size = (num_elements + block_size - 1) / block_size;

    // Allocate and initialize data with srand(42)
    srand(42);
    float *h_data = (float *)malloc(num_elements * sizeof(float));
    for (int i = 0; i < num_elements; i++) {
        h_data[i] = (float)(rand() % 1000) / 1000.0f;
    }

    float *d_data;
    HIP_CHECK(hipMalloc(&d_data, num_elements * sizeof(float)));
    HIP_CHECK(hipMemcpy(d_data, h_data, num_elements * sizeof(float),
                         hipMemcpyHostToDevice));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_elements);

    printCSVHeader();

    dim3 block(block_size);
    dim3 grid(grid_size);

    // Total FLOPs = num_elements * num_iterations * 8 (4 FMAs * 2 FLOPS each)
    double total_flops = (double)num_elements * (double)num_iterations * 8.0;

    BenchResult r = runBenchmark("occupancy_fma", problemSize, iterations, [&]() {
        hipLaunchKernelGGL(occupancy_fma_kernel, grid, block,
                           shared_mem_bytes, 0,
                           d_data, num_elements, num_iterations);
    });
    printCSVRow(r);

    double gflops = (total_flops / 1.0e9) / (r.avg_ms / 1000.0);
    printf("# Total FLOPs: %.2e\n", total_flops);
    printf("# Elements: %d, Block: %d, SharedMem: %d bytes, Iters: %d\n",
           num_elements, block_size, shared_mem_bytes, num_iterations);
    printf("# Throughput: %.2f GFLOPS\n", gflops);

    // Cleanup
    HIP_CHECK(hipFree(d_data));
    free(h_data);

    return 0;
}
