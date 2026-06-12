// atomic_throughput.cu -- CUDA benchmark for atomic operation throughput
// All threads perform atomicAdd to a global counter array, measuring
// contention effects. By varying num_counters, contention can be
// high (1 counter), medium (32), or low (1024).
// Reports atomic ops/sec.

#include "bench_common_cuda.h"

__global__ void atomic_throughput_kernel(int *counters, int num_counters,
                                         int num_atomics_per_thread) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;

    for (int i = 0; i < num_atomics_per_thread; i++) {
        atomicAdd(&counters[tid % num_counters], 1);
    }
}

int main(int argc, char **argv) {
    int iterations            = parseIterations(argc, argv);
    int num_threads           = parseIntParam(argc, argv, "--num_threads", 65536);
    int block_size            = parseIntParam(argc, argv, "--block_size", 256);
    int num_atomics_per_thread = parseIntParam(argc, argv, "--num_atomics_per_thread", 1024);
    int num_counters          = parseIntParam(argc, argv, "--num_counters", 1);

    int grid_size = (num_threads + block_size - 1) / block_size;
    int actual_threads = grid_size * block_size;

    // Allocate counter array on device
    int *d_counters;
    CUDA_CHECK(cudaMalloc(&d_counters, num_counters * sizeof(int)));
    CUDA_CHECK(cudaMemset(d_counters, 0, num_counters * sizeof(int)));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_threads);

    printCSVHeader();

    dim3 block(block_size);
    dim3 grid(grid_size);

    // Total atomic operations = actual_threads * num_atomics_per_thread
    double total_atomic_ops = (double)actual_threads * (double)num_atomics_per_thread;

    BenchResult r = runBenchmark("atomic_throughput", problemSize, iterations, [&]() {
        // Reset counters before each run
        CUDA_CHECK(cudaMemset(d_counters, 0, num_counters * sizeof(int)));
        atomic_throughput_kernel<<<grid, block>>>(d_counters, num_counters,
                                                   num_atomics_per_thread);
    });
    printCSVRow(r);

    double ops_per_sec = total_atomic_ops / (r.avg_ms / 1000.0);
    printf("# Total atomic ops: %.2e\n", total_atomic_ops);
    printf("# Threads: %d, Atomics/thread: %d, Counters: %d\n",
           actual_threads, num_atomics_per_thread, num_counters);
    printf("# Throughput: %.2f Gatomic_ops/sec\n", ops_per_sec / 1.0e9);

    // Cleanup
    CUDA_CHECK(cudaFree(d_counters));

    return 0;
}
