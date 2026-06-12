// global_bandwidth.cu -- CUDA benchmark for global memory bandwidth
// Measures global memory bandwidth via sequential read/write of large arrays.
// Kernel reads from one array and writes to another (copy pattern).

#include "bench_common_cuda.h"

// Copy kernel: d_out[gid] = d_in[gid]
__global__ void copy_kernel(const float *d_in, float *d_out, int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        d_out[gid] = d_in[gid];
    }
}

// Read-only kernel: accumulate reads to prevent optimization
__global__ void read_kernel(const float *d_in, float *d_out, int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float val = d_in[gid];
        if (val == -999.0f) d_out[0] = val; // prevent dead-code elimination
    }
}

// Write-only kernel: write pattern
__global__ void write_kernel(float *d_out, int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        d_out[gid] = (float)gid;
    }
}

// Strided copy kernel
__global__ void strided_copy_kernel(const float *d_in, float *d_out,
                                    int N, int stride) {
    int gid = (blockIdx.x * blockDim.x + threadIdx.x) * stride;
    if (gid < N) {
        d_out[gid] = d_in[gid];
    }
}

int main(int argc, char **argv) {
    int iterations  = parseIterations(argc, argv);
    int array_size  = parseIntParam(argc, argv, "--array_size", 67108864);
    int block_size  = parseIntParam(argc, argv, "--block_size", 256);
    int stride      = parseIntParam(argc, argv, "--stride", 1);
    int access_type = parseIntParam(argc, argv, "--access_type", 3); // 1=read, 2=write, 3=copy

    int N = array_size / (int)sizeof(float);

    // Generate host data
    float *h_in = (float *)malloc(array_size);
    srand(42);
    for (int i = 0; i < N; i++) {
        h_in[i] = (float)(rand() % 1000) / 100.0f;
    }

    // Device allocations
    float *d_in, *d_out;
    CUDA_CHECK(cudaMalloc(&d_in, array_size));
    CUDA_CHECK(cudaMalloc(&d_out, array_size));
    CUDA_CHECK(cudaMemcpy(d_in, h_in, array_size, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemset(d_out, 0, array_size));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", array_size);

    printCSVHeader();

    if (access_type == 1) {
        // Read-only
        dim3 block(block_size);
        dim3 grid((N + block_size - 1) / block_size);

        BenchResult r = runBenchmark("global_bw_read", problemSize, iterations, [&]() {
            read_kernel<<<grid, block>>>(d_in, d_out, N);
        });
        printCSVRow(r);

        double bytes = (double)array_size;
        double gbps = (bytes / 1.0e9) / (r.avg_ms / 1000.0);
        printf("# Throughput: %.2f GB/s (read)\n", gbps);
    } else if (access_type == 2) {
        // Write-only
        dim3 block(block_size);
        dim3 grid((N + block_size - 1) / block_size);

        BenchResult r = runBenchmark("global_bw_write", problemSize, iterations, [&]() {
            write_kernel<<<grid, block>>>(d_out, N);
        });
        printCSVRow(r);

        double bytes = (double)array_size;
        double gbps = (bytes / 1.0e9) / (r.avg_ms / 1000.0);
        printf("# Throughput: %.2f GB/s (write)\n", gbps);
    } else {
        // Copy (read + write)
        if (stride > 1) {
            int effective_N = N;
            int threads = (effective_N + stride - 1) / stride;
            dim3 block(block_size);
            dim3 grid((threads + block_size - 1) / block_size);

            BenchResult r = runBenchmark("global_bw_strided_copy", problemSize, iterations, [&]() {
                strided_copy_kernel<<<grid, block>>>(d_in, d_out, effective_N, stride);
            });
            printCSVRow(r);

            double bytes = 2.0 * (double)threads * sizeof(float);
            double gbps = (bytes / 1.0e9) / (r.avg_ms / 1000.0);
            printf("# Throughput: %.2f GB/s (strided copy, stride=%d)\n", gbps, stride);
        } else {
            dim3 block(block_size);
            dim3 grid((N + block_size - 1) / block_size);

            BenchResult r = runBenchmark("global_bw_copy", problemSize, iterations, [&]() {
                copy_kernel<<<grid, block>>>(d_in, d_out, N);
            });
            printCSVRow(r);

            double bytes = 2.0 * (double)array_size;
            double gbps = (bytes / 1.0e9) / (r.avg_ms / 1000.0);
            printf("# Throughput: %.2f GB/s (copy)\n", gbps);
        }
    }

    // Cleanup
    CUDA_CHECK(cudaFree(d_in));
    CUDA_CHECK(cudaFree(d_out));
    free(h_in);

    return 0;
}
