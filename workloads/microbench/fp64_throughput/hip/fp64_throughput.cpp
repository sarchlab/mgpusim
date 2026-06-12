// fp64_throughput.cpp -- HIP benchmark for double-precision floating-point throughput
// Each thread performs a massive number of FP64 add/multiply/FMA operations
// in a tight dependent loop. Pure compute bound -- no memory dependencies.
// Reports GFLOPS (giga floating-point operations per second).

#include "bench_common_hip.h"

// FP64 add throughput kernel
__global__ void fp64_add_kernel(int num_ops, double *d_out) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    double a = (double)(tid + 1) * 1.00000000001;
    double b = 0.99999999999;

    for (int i = 0; i < num_ops; i++) {
        a = a + b;
        b = b + a;
        a = a + b;
        b = b + a;
    }

    // Prevent dead-code elimination
    if (a == -999.0) d_out[tid] = a + b;
}

// FP64 multiply throughput kernel
__global__ void fp64_mul_kernel(int num_ops, double *d_out) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    double a = (double)(tid + 1) * 1.00000000001;
    double b = 1.00000000001;

    for (int i = 0; i < num_ops; i++) {
        a = a * b;
        b = b * a;
        a = a * b;
        b = b * a;
    }

    if (a == -999.0) d_out[tid] = a + b;
}

// FP64 FMA (fused multiply-add) throughput kernel
__global__ void fp64_fma_kernel(int num_ops, double *d_out) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    double a = (double)(tid + 1) * 1.00000000001;
    double b = 1.00000000001;
    double c = 0.99999999999;

    for (int i = 0; i < num_ops; i++) {
        a = a * b + c;
        b = b * a + c;
        a = a * c + b;
        b = b * c + a;
    }

    if (a == -999.0) d_out[tid] = a + b;
}

int main(int argc, char **argv) {
    int iterations        = parseIterations(argc, argv);
    int num_ops           = parseIntParam(argc, argv, "--num_ops", 1048576);
    int block_size        = parseIntParam(argc, argv, "--block_size", 256);
    int op_type           = parseIntParam(argc, argv, "--op_type", 3); // 1=add, 2=mul, 3=fma
    int num_threads_total = parseIntParam(argc, argv, "--num_threads_total", 65536);

    int grid_size = (num_threads_total + block_size - 1) / block_size;

    // Allocate small output buffer (just to prevent DCE)
    double *d_out;
    HIP_CHECK(hipMalloc(&d_out, num_threads_total * sizeof(double)));
    HIP_CHECK(hipMemset(d_out, 0, num_threads_total * sizeof(double)));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_ops);

    printCSVHeader();

    dim3 block(block_size);
    dim3 grid(grid_size);

    long long total_threads = (long long)grid_size * block_size;

    if (op_type == 1) {
        // Add: 4 adds per iteration
        long long flops_per_thread = (long long)num_ops * 4;
        double total_flops = (double)flops_per_thread * (double)total_threads;

        BenchResult r = runBenchmark("fp64_add", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(fp64_add_kernel, grid, block, 0, 0,
                               num_ops, d_out);
        });
        printCSVRow(r);

        double gflops = (total_flops / 1.0e9) / (r.avg_ms / 1000.0);
        printf("# Total FLOPs: %.2e\n", total_flops);
        printf("# Throughput: %.2f GFLOPS (fp64 add)\n", gflops);
    } else if (op_type == 2) {
        // Mul: 4 multiplies per iteration
        long long flops_per_thread = (long long)num_ops * 4;
        double total_flops = (double)flops_per_thread * (double)total_threads;

        BenchResult r = runBenchmark("fp64_mul", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(fp64_mul_kernel, grid, block, 0, 0,
                               num_ops, d_out);
        });
        printCSVRow(r);

        double gflops = (total_flops / 1.0e9) / (r.avg_ms / 1000.0);
        printf("# Total FLOPs: %.2e\n", total_flops);
        printf("# Throughput: %.2f GFLOPS (fp64 mul)\n", gflops);
    } else {
        // FMA: each statement is a*b+c = 2 FLOPs, 4 statements = 8 FLOPs per iteration
        long long flops_per_thread = (long long)num_ops * 8;
        double total_flops = (double)flops_per_thread * (double)total_threads;

        BenchResult r = runBenchmark("fp64_fma", problemSize, iterations, [&]() {
            hipLaunchKernelGGL(fp64_fma_kernel, grid, block, 0, 0,
                               num_ops, d_out);
        });
        printCSVRow(r);

        double gflops = (total_flops / 1.0e9) / (r.avg_ms / 1000.0);
        printf("# Total FLOPs: %.2e\n", total_flops);
        printf("# Throughput: %.2f GFLOPS (fp64 fma)\n", gflops);
    }

    // Cleanup
    HIP_CHECK(hipFree(d_out));

    return 0;
}
