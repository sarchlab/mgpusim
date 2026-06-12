// int_throughput.cpp -- HIP benchmark for integer arithmetic throughput
// Each thread performs a massive number of integer add/multiply/MAD operations
// in a tight dependent loop. Pure ALU bound -- no memory dependencies.
// Reports GIOPS (giga integer operations per second).

#include "bench_common_hip.h"

// Integer add throughput kernel
__global__ void int_add_kernel(int num_ops, int *d_out) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int a = tid + 1;
    int b = 3;

    for (int i = 0; i < num_ops; i++) {
        a = a + b;
        b = b + a;
        a = a + b;
        b = b + a;
    }

    // Prevent dead-code elimination
    if (a == -999) d_out[tid] = a + b;
}

// Integer multiply throughput kernel
__global__ void int_mul_kernel(int num_ops, int *d_out) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int a = tid + 1;
    int b = 3;

    for (int i = 0; i < num_ops; i++) {
        a = a * b + 1; // +1 to prevent convergence to 0
        b = b * a + 1;
        a = a * b + 1;
        b = b * a + 1;
    }

    if (a == -999) d_out[tid] = a + b;
}

// Integer MAD (multiply-add) throughput kernel
__global__ void int_mad_kernel(int num_ops, int *d_out) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int a = tid + 1;
    int b = 3;
    int c = 7;

    for (int i = 0; i < num_ops; i++) {
        a = a * b + c;
        b = b * a + c;
        a = a * c + b;
        b = b * c + a;
    }

    if (a == -999) d_out[tid] = a + b;
}

int main(int argc, char **argv) {
    int iterations       = parseIterations(argc, argv);
    int num_ops          = parseIntParam(argc, argv, "--num_ops", 1048576);
    int block_size       = parseIntParam(argc, argv, "--block_size", 256);
    int op_type          = parseIntParam(argc, argv, "--op_type", 3); // 1=add, 2=mul, 3=mad
    int num_threads_total = parseIntParam(argc, argv, "--num_threads_total", 65536);

    int grid_size = (num_threads_total + block_size - 1) / block_size;

    // Allocate small output buffer (just to prevent DCE)
    int *d_out;
    HIP_CHECK(hipMalloc(&d_out, num_threads_total * sizeof(int)));
    HIP_CHECK(hipMemset(d_out, 0, num_threads_total * sizeof(int)));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_ops);

    printCSVHeader();

    dim3 block(block_size);
    dim3 grid(grid_size);

    // Each loop iteration does 4 operations
    long long ops_per_thread = (long long)num_ops * 4;
    long long total_threads  = (long long)grid_size * block_size;
    double total_ops         = (double)ops_per_thread * (double)total_threads;

    if (op_type == 1) {
        BenchResult r = runBenchmark("int_add", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(int_add_kernel, grid, block, 0, 0,
                               num_ops, d_out);
        });
        printCSVRow(r);

        double giops = (total_ops / 1.0e9) / (r.avg_ms / 1000.0);
        printf("# Total ops: %.2e\n", total_ops);
        printf("# Throughput: %.2f GIOPS (int add)\n", giops);
    } else if (op_type == 2) {
        // mul kernel does mad (multiply + add), so 2 ops per statement, 4 statements = 8 ops
        long long mul_ops_per_thread = (long long)num_ops * 8;
        double mul_total_ops = (double)mul_ops_per_thread * (double)total_threads;

        BenchResult r = runBenchmark("int_mul", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(int_mul_kernel, grid, block, 0, 0,
                               num_ops, d_out);
        });
        printCSVRow(r);

        double giops = (mul_total_ops / 1.0e9) / (r.avg_ms / 1000.0);
        printf("# Total ops: %.2e\n", mul_total_ops);
        printf("# Throughput: %.2f GIOPS (int mul+add)\n", giops);
    } else {
        // mad: each statement is a * b + c = 2 ops, 4 statements = 8 ops per iteration
        long long mad_ops_per_thread = (long long)num_ops * 8;
        double mad_total_ops = (double)mad_ops_per_thread * (double)total_threads;

        BenchResult r = runBenchmark("int_mad", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(int_mad_kernel, grid, block, 0, 0,
                               num_ops, d_out);
        });
        printCSVRow(r);

        double giops = (mad_total_ops / 1.0e9) / (r.avg_ms / 1000.0);
        printf("# Total ops: %.2e\n", mad_total_ops);
        printf("# Throughput: %.2f GIOPS (int mad)\n", giops);
    }

    // Cleanup
    HIP_CHECK(hipFree(d_out));

    return 0;
}
