// micro_membw.cpp — Memory bandwidth microbenchmark for AMD MI300A (gfx942)
//
// Measures effective global memory bandwidth via three streaming patterns:
//   1. Stream Read  — sum all elements (forces read from DRAM)
//   2. Stream Write — fill all elements with a constant
//   3. Stream Copy  — copy array A to array B
//
// Build:  hipcc -O2 micro_membw.cpp -o micro_membw
// Run:    ./micro_membw [--iterations N]
//
// Output: CSV with bandwidth in GB/s for each operation and array size.

#include "bench_common.h"
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>

// ---------------------------------------------------------------------------
// Kernels
// ---------------------------------------------------------------------------

// Stream read: each thread reads elements and accumulates into partial sum
__global__ void stream_read_kernel(const float* __restrict__ src,
                                   float* __restrict__ partial_sums,
                                   size_t N) {
    size_t tid = blockIdx.x * (size_t)blockDim.x + threadIdx.x;
    size_t stride = (size_t)blockDim.x * gridDim.x;
    float sum = 0.0f;
    for (size_t i = tid; i < N; i += stride) {
        sum += src[i];
    }
    partial_sums[tid] = sum;
}

// Stream write: each thread writes a constant value to dst
__global__ void stream_write_kernel(float* __restrict__ dst,
                                    float val,
                                    size_t N) {
    size_t tid = blockIdx.x * (size_t)blockDim.x + threadIdx.x;
    size_t stride = (size_t)blockDim.x * gridDim.x;
    for (size_t i = tid; i < N; i += stride) {
        dst[i] = val;
    }
}

// Stream copy: copy src to dst
__global__ void stream_copy_kernel(const float* __restrict__ src,
                                   float* __restrict__ dst,
                                   size_t N) {
    size_t tid = blockIdx.x * (size_t)blockDim.x + threadIdx.x;
    size_t stride = (size_t)blockDim.x * gridDim.x;
    for (size_t i = tid; i < N; i += stride) {
        dst[i] = src[i];
    }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

static const char* bytes_to_label(size_t bytes) {
    static char buf[64];
    if (bytes >= (1ULL << 30))
        snprintf(buf, sizeof(buf), "%zuGB", bytes >> 30);
    else
        snprintf(buf, sizeof(buf), "%zuMB", bytes >> 20);
    return buf;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int iters = parseIterations(argc, argv);

    // Array sizes to test
    const size_t sizes[] = {
        256ULL * 1024 * 1024,   // 256 MB
        512ULL * 1024 * 1024,   // 512 MB
        1024ULL * 1024 * 1024   // 1 GB
    };
    const int num_sizes = sizeof(sizes) / sizeof(sizes[0]);

    const int blockSize = 256;
    const int numBlocks = 1024;  // plenty of work for MI300A
    const size_t totalThreads = (size_t)blockSize * numBlocks;

    printf("# Memory Bandwidth Microbenchmark (MI300A)\n");
    printf("# Iterations per test: %d\n", iters);
    printf("#\n");
    printf("operation,array_size,iterations,avg_ms,min_ms,max_ms,avg_gbps\n");

    for (int si = 0; si < num_sizes; si++) {
        size_t byteSize = sizes[si];
        size_t N = byteSize / sizeof(float);
        const char* sizeLabel = bytes_to_label(byteSize);

        // Allocate device memory
        float* d_A = nullptr;
        float* d_B = nullptr;
        float* d_partial = nullptr;

        HIP_CHECK(hipMalloc(&d_A, byteSize));
        HIP_CHECK(hipMalloc(&d_B, byteSize));
        HIP_CHECK(hipMalloc(&d_partial, totalThreads * sizeof(float)));

        // Initialize d_A with some data
        HIP_CHECK(hipMemset(d_A, 1, byteSize));
        HIP_CHECK(hipMemset(d_B, 0, byteSize));
        HIP_CHECK(hipDeviceSynchronize());

        // --- Stream Read ---
        {
            char psize[128];
            snprintf(psize, sizeof(psize), "%s", sizeLabel);

            BenchResult r = runBenchmark("stream_read", psize, iters, [&]() {
                hipLaunchKernelGGL(stream_read_kernel,
                                   dim3(numBlocks), dim3(blockSize), 0, 0,
                                   d_A, d_partial, N);
            });

            // Bytes read: N floats
            double bytes_transferred = (double)byteSize;
            double avg_gbps = (bytes_transferred / (1.0e6 * r.avg_ms));  // GB/s

            printf("stream_read,%s,%d,%.4f,%.4f,%.4f,%.2f\n",
                   psize, r.iterations, r.avg_ms, r.min_ms, r.max_ms, avg_gbps);
        }

        // --- Stream Write ---
        {
            char psize[128];
            snprintf(psize, sizeof(psize), "%s", sizeLabel);

            BenchResult r = runBenchmark("stream_write", psize, iters, [&]() {
                hipLaunchKernelGGL(stream_write_kernel,
                                   dim3(numBlocks), dim3(blockSize), 0, 0,
                                   d_B, 3.14f, N);
            });

            double bytes_transferred = (double)byteSize;
            double avg_gbps = (bytes_transferred / (1.0e6 * r.avg_ms));

            printf("stream_write,%s,%d,%.4f,%.4f,%.4f,%.2f\n",
                   psize, r.iterations, r.avg_ms, r.min_ms, r.max_ms, avg_gbps);
        }

        // --- Stream Copy ---
        {
            char psize[128];
            snprintf(psize, sizeof(psize), "%s", sizeLabel);

            BenchResult r = runBenchmark("stream_copy", psize, iters, [&]() {
                hipLaunchKernelGGL(stream_copy_kernel,
                                   dim3(numBlocks), dim3(blockSize), 0, 0,
                                   d_A, d_B, N);
            });

            // Read N + Write N = 2N floats
            double bytes_transferred = 2.0 * (double)byteSize;
            double avg_gbps = (bytes_transferred / (1.0e6 * r.avg_ms));

            printf("stream_copy,%s,%d,%.4f,%.4f,%.4f,%.2f\n",
                   psize, r.iterations, r.avg_ms, r.min_ms, r.max_ms, avg_gbps);
        }

        // Cleanup
        HIP_CHECK(hipFree(d_A));
        HIP_CHECK(hipFree(d_B));
        HIP_CHECK(hipFree(d_partial));
    }

    printf("#\n");
    printf("# Done.\n");
    return 0;
}
