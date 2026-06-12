// l1_cache_bw.cu -- CUDA benchmark for L1 cache bandwidth
// Measures L1 cache bandwidth with a work-amount problem-size axis. The selected
// problem_size is num_elements (output elements/threads), while working_set_size
// remains a non-scaling L1-resident footprint for this CUDA path. The Go
// simulator sample calls the repeat count num_repeats and uses per-256-thread
// read-group reuse rather than a fixed total L1 working_set_size.
// Formula: total_read_bytes = num_elements * num_iterations * sizeof(float).

#include "bench_common_cuda.h"

// L1 cache bandwidth kernel: repeated reads from a small working set.
__global__ void l1_cache_bw_kernel(const float *d_data, float *d_out,
                                   int working_set_elems, int num_iters,
                                   int stride, int total_threads) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid >= total_threads) return;

    float accum = 0.0f;
    int idx = gid % working_set_elems;

    for (int iter = 0; iter < num_iters; iter++) {
        accum += d_data[idx];
        idx = (idx + stride) % working_set_elems;
    }

    // Write result to prevent dead-code elimination.
    d_out[gid] = accum;
}

int main(int argc, char **argv) {
    int iterations       = parseIterations(argc, argv);
    int num_elements     = parseIntParam(argc, argv, "--num_elements", 65536);
    int working_set_size = parseIntParam(argc, argv, "--working_set_size", 8192);
    int block_size       = parseIntParam(argc, argv, "--block_size", 256);
    int num_iters        = parseIntParam(argc, argv, "--num_iterations", 1000);
    int stride           = parseIntParam(argc, argv, "--stride", 1);

    // --total_threads is kept as a compatibility alias for direct invocations.
    int total_threads = parseIntParam(argc, argv, "--total_threads", num_elements);

    int working_set_elems = working_set_size / (int)sizeof(float);
    if (working_set_elems < 1) working_set_elems = 1;

    // Generate host data for the fixed/non-scaling cache-residency footprint.
    float *h_data = (float *)malloc(working_set_size);
    srand(42);
    for (int i = 0; i < working_set_elems; i++) {
        h_data[i] = (float)(rand() % 1000) / 100.0f;
    }

    // Device allocations.
    float *d_data, *d_out;
    CUDA_CHECK(cudaMalloc(&d_data, working_set_size));
    CUDA_CHECK(cudaMalloc(&d_out, total_threads * sizeof(float)));
    CUDA_CHECK(cudaMemcpy(d_data, h_data, working_set_size,
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemset(d_out, 0, total_threads * sizeof(float)));

    dim3 block(block_size);
    dim3 grid((total_threads + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", total_threads);

    printCSVHeader();

    BenchResult r = runBenchmark("l1_cache_bw", problemSize, iterations, [&]() {
        l1_cache_bw_kernel<<<grid, block>>>(d_data, d_out,
                                            working_set_elems, num_iters,
                                            stride, total_threads);
    });
    printCSVRow(r);

    // Formula for selected problem_size axis:
    //   total_read_bytes = num_elements * num_iterations * sizeof(float)
    double total_bytes = (double)total_threads * (double)num_iters * sizeof(float);
    double gbps = (total_bytes / 1.0e9) / (r.avg_ms / 1000.0);
    printf("# L1 cache throughput: %.2f GB/s "
           "(num_elements=%d, total_read_bytes=%.0f, "
           "working_set=%d bytes, iters=%d, stride=%d)\n",
           gbps, total_threads, total_bytes, working_set_size, num_iters, stride);

    // Cleanup.
    CUDA_CHECK(cudaFree(d_data));
    CUDA_CHECK(cudaFree(d_out));
    free(h_data);

    return 0;
}
