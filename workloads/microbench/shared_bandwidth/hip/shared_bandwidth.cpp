// shared_bandwidth.cpp -- HIP benchmark for shared memory bandwidth
// Measures shared memory bandwidth with configurable bank conflict patterns.
// Kernel loads from global to shared, performs strided shared memory accesses,
// then stores back to global memory.

#include "bench_common_hip.h"

// Shared memory bandwidth kernel with configurable stride for bank conflicts
__global__ void shared_bw_kernel(const float *d_in, float *d_out,
                                 int N, int bank_stride, int num_iters) {
    extern __shared__ float smem[];

    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int tid = hipThreadIdx_x;
    int block_elems = hipBlockDim_x;

    // Load from global to shared memory
    if (gid < N) {
        smem[tid] = d_in[gid];
    } else {
        smem[tid] = 0.0f;
    }
    __syncthreads();

    // Perform repeated strided accesses in shared memory
    float accum = 0.0f;
    for (int iter = 0; iter < num_iters; iter++) {
        int src_idx = (tid * bank_stride) % block_elems;
        accum += smem[src_idx];
        __syncthreads();
        smem[tid] = accum;
        __syncthreads();
    }

    // Store result back to global memory
    if (gid < N) {
        d_out[gid] = accum;
    }
}

int main(int argc, char **argv) {
    int iterations  = parseIterations(argc, argv);
    int array_size  = parseIntParam(argc, argv, "--array_size", 67108864);
    int block_size  = parseIntParam(argc, argv, "--block_size", 256);
    int bank_stride = parseIntParam(argc, argv, "--bank_stride", 1);
    int num_iters   = parseIntParam(argc, argv, "--num_iterations", 100);

    int N = array_size / (int)sizeof(float);

    // Generate host data
    float *h_in = (float *)malloc(array_size);
    srand(42);
    for (int i = 0; i < N; i++) {
        h_in[i] = (float)(rand() % 1000) / 100.0f;
    }

    // Device allocations
    float *d_in, *d_out;
    HIP_CHECK(hipMalloc(&d_in, array_size));
    HIP_CHECK(hipMalloc(&d_out, array_size));
    HIP_CHECK(hipMemcpy(d_in, h_in, array_size, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemset(d_out, 0, array_size));

    dim3 block(block_size);
    dim3 grid((N + block_size - 1) / block_size);
    int shared_mem_size = block_size * sizeof(float);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", array_size);

    printCSVHeader();

    BenchResult r = runBenchmark("shared_bw", problemSize, iterations, [&]() {
        hipLaunchKernelGGL(shared_bw_kernel, grid, block, shared_mem_size, 0,
                           d_in, d_out, N, bank_stride, num_iters);
    });
    printCSVRow(r);

    // Each iteration: block_size reads + block_size writes in shared memory
    // Total shared mem bytes = N * num_iters * 2 * sizeof(float)
    double shared_bytes = 2.0 * (double)N * (double)num_iters * sizeof(float);
    double gbps = (shared_bytes / 1.0e9) / (r.avg_ms / 1000.0);
    printf("# Shared mem throughput: %.2f GB/s (bank_stride=%d, iters=%d)\n",
           gbps, bank_stride, num_iters);

    // Cleanup
    HIP_CHECK(hipFree(d_in));
    HIP_CHECK(hipFree(d_out));
    free(h_in);

    return 0;
}
