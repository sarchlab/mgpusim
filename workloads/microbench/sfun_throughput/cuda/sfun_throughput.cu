// sfun_throughput.cu -- CUDA benchmark for special function unit throughput
// Each thread computes sin/cos/exp/sqrt/rsqrt in a tight loop.
// Pure compute bound -- measures SFU throughput.
// Reports Gops/sec (giga operations per second).

#include "bench_common_cuda.h"

// Sin throughput kernel
__global__ void sfun_sin_kernel(int num_ops, float *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    float val = (float)(tid + 1) * 0.0001f;

    for (int i = 0; i < num_ops; i++) {
        val = sinf(val);
        val = sinf(val);
        val = sinf(val);
        val = sinf(val);
    }

    // Prevent dead-code elimination
    if (val == -999.0f) d_out[tid] = val;
}

// Cos throughput kernel
__global__ void sfun_cos_kernel(int num_ops, float *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    float val = (float)(tid + 1) * 0.0001f;

    for (int i = 0; i < num_ops; i++) {
        val = cosf(val);
        val = cosf(val);
        val = cosf(val);
        val = cosf(val);
    }

    if (val == -999.0f) d_out[tid] = val;
}

// Exp throughput kernel
__global__ void sfun_exp_kernel(int num_ops, float *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    float val = (float)(tid + 1) * 0.0000001f;

    for (int i = 0; i < num_ops; i++) {
        val = expf(val);
        val = expf(val);
        val = expf(val);
        val = expf(val);
    }

    if (val == -999.0f) d_out[tid] = val;
}

// Sqrt throughput kernel
__global__ void sfun_sqrt_kernel(int num_ops, float *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    float val = (float)(tid + 1) * 1.0001f + 1.0f;

    for (int i = 0; i < num_ops; i++) {
        val = sqrtf(val);
        val = sqrtf(val);
        val = sqrtf(val);
        val = sqrtf(val);
    }

    if (val == -999.0f) d_out[tid] = val;
}

// Rsqrt throughput kernel
__global__ void sfun_rsqrt_kernel(int num_ops, float *d_out) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    float val = (float)(tid + 1) * 1.0001f + 1.0f;

    for (int i = 0; i < num_ops; i++) {
        val = rsqrtf(val);
        val = rsqrtf(val);
        val = rsqrtf(val);
        val = rsqrtf(val);
    }

    if (val == -999.0f) d_out[tid] = val;
}

int main(int argc, char **argv) {
    int iterations        = parseIterations(argc, argv);
    int num_ops           = parseIntParam(argc, argv, "--num_ops", 1048576);
    int block_size        = parseIntParam(argc, argv, "--block_size", 256);
    int func_type         = parseIntParam(argc, argv, "--func_type", 1); // 1=sin, 2=cos, 3=exp, 4=sqrt, 5=rsqrt
    int num_threads_total = parseIntParam(argc, argv, "--num_threads_total", 65536);

    int grid_size = (num_threads_total + block_size - 1) / block_size;

    // Allocate small output buffer (just to prevent DCE)
    float *d_out;
    CUDA_CHECK(cudaMalloc(&d_out, num_threads_total * sizeof(float)));
    CUDA_CHECK(cudaMemset(d_out, 0, num_threads_total * sizeof(float)));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_ops);

    printCSVHeader();

    dim3 block(block_size);
    dim3 grid(grid_size);

    long long total_threads = (long long)grid_size * block_size;
    // 4 operations per iteration
    long long ops_per_thread = (long long)num_ops * 4;
    double total_ops = (double)ops_per_thread * (double)total_threads;

    const char *func_name;
    switch (func_type) {
        case 1: func_name = "sin"; break;
        case 2: func_name = "cos"; break;
        case 3: func_name = "exp"; break;
        case 4: func_name = "sqrt"; break;
        case 5: func_name = "rsqrt"; break;
        default: func_name = "sin"; func_type = 1; break;
    }

    char kernel_name[64];
    snprintf(kernel_name, sizeof(kernel_name), "sfun_%s", func_name);

    BenchResult r;
    switch (func_type) {
        case 1:
            r = runBenchmark(kernel_name, problemSize, iterations, [&]() {
                sfun_sin_kernel<<<grid, block>>>(num_ops, d_out);
            });
            break;
        case 2:
            r = runBenchmark(kernel_name, problemSize, iterations, [&]() {
                sfun_cos_kernel<<<grid, block>>>(num_ops, d_out);
            });
            break;
        case 3:
            r = runBenchmark(kernel_name, problemSize, iterations, [&]() {
                sfun_exp_kernel<<<grid, block>>>(num_ops, d_out);
            });
            break;
        case 4:
            r = runBenchmark(kernel_name, problemSize, iterations, [&]() {
                sfun_sqrt_kernel<<<grid, block>>>(num_ops, d_out);
            });
            break;
        case 5:
            r = runBenchmark(kernel_name, problemSize, iterations, [&]() {
                sfun_rsqrt_kernel<<<grid, block>>>(num_ops, d_out);
            });
            break;
    }
    printCSVRow(r);

    double gops = (total_ops / 1.0e9) / (r.avg_ms / 1000.0);
    printf("# Total ops: %.2e\n", total_ops);
    printf("# Throughput: %.2f Gops/sec (%s)\n", gops, func_name);

    // Cleanup
    CUDA_CHECK(cudaFree(d_out));

    return 0;
}
