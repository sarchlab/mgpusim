// reduction.cpp -- SHOC Reduction benchmark (HIP)
// Parallel sum reduction of a float array.
// Two-pass: reduce to partial sums, then reduce partials.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Kernel: shared-memory tree reduction
// Each block reduces 2*blockDim.x elements to one partial sum.
// ---------------------------------------------------------------------------
__global__ void reduce_kernel(const float* __restrict__ input,
                              float* __restrict__ output,
                              int N) {
    extern __shared__ float sdata[];

    unsigned int tid = threadIdx.x;
    unsigned int i = blockIdx.x * blockDim.x * 2 + threadIdx.x;

    // Load two elements per thread into shared memory
    float val = 0.0f;
    if (i < N)
        val = input[i];
    if (i + blockDim.x < N)
        val += input[i + blockDim.x];
    sdata[tid] = val;
    __syncthreads();

    // Tree reduction in shared memory
    for (unsigned int s = blockDim.x / 2; s > 0; s >>= 1) {
        if (tid < s) {
            sdata[tid] += sdata[tid + s];
        }
        __syncthreads();
    }

    // Write result for this block
    if (tid == 0) {
        output[blockIdx.x] = sdata[0];
    }
}

// ---------------------------------------------------------------------------
// CPU reference for verification
// ---------------------------------------------------------------------------
static double reduction_cpu(const float* data, int N) {
    double sum = 0.0;
    for (int i = 0; i < N; ++i) {
        sum += (double)data[i];
    }
    return sum;
}

// ---------------------------------------------------------------------------
// Two-pass reduction helper
// ---------------------------------------------------------------------------
static void gpu_reduce(const float* d_input, float* d_partial,
                       float* d_result, float* h_result,
                       int N, int block_size) {
    int elems_per_block = block_size * 2;
    int num_blocks = (N + elems_per_block - 1) / elems_per_block;
    int shared_bytes = block_size * sizeof(float);

    // Pass 1: reduce input to partial sums
    hipLaunchKernelGGL(reduce_kernel, dim3(num_blocks), dim3(block_size),
                       shared_bytes, 0, d_input, d_partial, N);

    // Pass 2: reduce partial sums to a single value
    if (num_blocks > 1) {
        int num_blocks2 = (num_blocks + elems_per_block - 1) / elems_per_block;
        if (num_blocks2 == 1) {
            hipLaunchKernelGGL(reduce_kernel, dim3(1), dim3(block_size),
                               shared_bytes, 0, d_partial, d_result,
                               num_blocks);
        } else {
            // Multi-level: keep reducing
            float* d_temp;
            HIP_CHECK(hipMalloc(&d_temp, num_blocks2 * sizeof(float)));
            hipLaunchKernelGGL(reduce_kernel, dim3(num_blocks2),
                               dim3(block_size), shared_bytes, 0,
                               d_partial, d_temp, num_blocks);
            // Final pass
            hipLaunchKernelGGL(reduce_kernel, dim3(1), dim3(block_size),
                               shared_bytes, 0, d_temp, d_result,
                               num_blocks2);
            HIP_CHECK(hipFree(d_temp));
        }
    } else {
        // Already a single block -- result is in d_partial[0]
        HIP_CHECK(hipMemcpy(d_result, d_partial, sizeof(float),
                             hipMemcpyDeviceToDevice));
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--size", 1048576);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int iters      = parseIterations(argc, argv);

    size_t bytesIn = (size_t)N * sizeof(float);

    // Host allocations
    float* h_input = (float*)malloc(bytesIn);

    // Initialize with known pattern
    for (int i = 0; i < N; ++i) {
        h_input[i] = (float)(i % 1000) * 0.001f;
    }

    // Device allocations
    float *d_input, *d_partial, *d_result;
    int elems_per_block = block_size * 2;
    int num_blocks = (N + elems_per_block - 1) / elems_per_block;

    HIP_CHECK(hipMalloc(&d_input, bytesIn));
    HIP_CHECK(hipMalloc(&d_partial, num_blocks * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_result, sizeof(float)));

    HIP_CHECK(hipMemcpy(d_input, h_input, bytesIn, hipMemcpyHostToDevice));

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("reduction", size_str, iters, [&]() {
        gpu_reduce(d_input, d_partial, d_result, nullptr, N, block_size);
    });

    printCSVRow(r);

    // Bandwidth: read N floats
    double bandwidth_gb = (double)N * sizeof(float) / (r.avg_ms * 1e-3) / 1e9;
    fprintf(stderr, "Effective bandwidth: %.2f GB/s\n", bandwidth_gb);

    // Verify
    float gpu_result;
    HIP_CHECK(hipMemcpy(&gpu_result, d_result, sizeof(float),
                         hipMemcpyDeviceToHost));
    double cpu_result = reduction_cpu(h_input, N);

    double rel_err = fabs((double)gpu_result - cpu_result) /
                     (fabs(cpu_result) + 1e-10);
    fprintf(stderr, "GPU sum: %f, CPU sum: %f, relative error: %e\n",
            gpu_result, cpu_result, rel_err);

    if (rel_err < 1e-3) {
        fprintf(stderr, "PASS\n");
    } else {
        fprintf(stderr, "FAIL: relative error too large\n");
    }

    // Cleanup
    HIP_CHECK(hipFree(d_input));
    HIP_CHECK(hipFree(d_partial));
    HIP_CHECK(hipFree(d_result));
    free(h_input);

    return (rel_err >= 1e-3) ? 1 : 0;
}
