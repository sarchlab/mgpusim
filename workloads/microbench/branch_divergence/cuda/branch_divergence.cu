// branch_divergence.cu -- CUDA benchmark for branch divergence penalty measurement
// Kernel has an if/else branch controlled by divergence percentage.
// A parameter controls what fraction of threads in each warp take the "if" branch.
// Reports effective throughput vs. no-divergence baseline.

#include "bench_common_cuda.h"

// Branch divergence kernel
// divergence_threshold: number of lanes (0-32) that take the "if" branch
// work_per_branch: number of iterations of work inside each branch
__global__ void branch_divergence_kernel(float *d_data, int num_elements,
                                          int divergence_threshold,
                                          int work_per_branch) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid >= num_elements) return;

    int warp_size = 32;
    int lane = tid % warp_size;
    float val = d_data[tid];

    if (lane < divergence_threshold) {
        // Branch A: multiply-add chain
        for (int i = 0; i < work_per_branch; i++) {
            val = val * 1.0001f + 0.9999f;
            val = val * 0.9999f + 1.0001f;
            val = val * 1.0001f + 0.9999f;
            val = val * 0.9999f + 1.0001f;
        }
    } else {
        // Branch B: add-multiply chain (different instruction mix)
        for (int i = 0; i < work_per_branch; i++) {
            val = val + 0.0001f;
            val = val * 1.0001f;
            val = val + 0.0001f;
            val = val * 1.0001f;
        }
    }

    d_data[tid] = val;
}

int main(int argc, char **argv) {
    int iterations       = parseIterations(argc, argv);
    int num_elements     = parseIntParam(argc, argv, "--num_elements", 1048576);
    int block_size       = parseIntParam(argc, argv, "--block_size", 256);
    int divergence_pct   = parseIntParam(argc, argv, "--divergence_pct", 50);
    int work_per_branch  = parseIntParam(argc, argv, "--work_per_branch", 1024);

    // Convert divergence percentage to threshold (lanes 0..threshold-1 take branch A)
    int warp_size = 32;
    int divergence_threshold = (divergence_pct * warp_size + 50) / 100;
    // Clamp to valid range
    if (divergence_threshold < 0) divergence_threshold = 0;
    if (divergence_threshold > warp_size) divergence_threshold = warp_size;

    int grid_size = (num_elements + block_size - 1) / block_size;

    // Allocate and initialize data
    float *h_data = (float *)malloc(num_elements * sizeof(float));
    srand(42);
    for (int i = 0; i < num_elements; i++) {
        h_data[i] = (float)(rand() % 1000) * 0.001f + 1.0f;
    }

    float *d_data;
    CUDA_CHECK(cudaMalloc(&d_data, num_elements * sizeof(float)));
    CUDA_CHECK(cudaMemcpy(d_data, h_data, num_elements * sizeof(float),
                          cudaMemcpyHostToDevice));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_elements);

    printCSVHeader();

    dim3 block(block_size);
    dim3 grid(grid_size);

    char kernel_name[64];
    snprintf(kernel_name, sizeof(kernel_name), "branch_div_%dpct", divergence_pct);

    BenchResult r = runBenchmark(kernel_name, problemSize, iterations, [&]() {
        // Re-upload data each iteration for consistent results
        CUDA_CHECK(cudaMemcpy(d_data, h_data, num_elements * sizeof(float),
                              cudaMemcpyHostToDevice));
        branch_divergence_kernel<<<grid, block>>>(d_data, num_elements,
                                                   divergence_threshold,
                                                   work_per_branch);
    });
    printCSVRow(r);

    // Report throughput
    // Each thread does work_per_branch iterations * 4 ops per iteration = 4*work_per_branch ops
    long long ops_per_thread = (long long)work_per_branch * 4;
    long long total_threads = (long long)num_elements;
    double total_ops = (double)ops_per_thread * (double)total_threads;
    double gops = (total_ops / 1.0e9) / (r.avg_ms / 1000.0);

    printf("# Divergence: %d%% (threshold=%d/%d lanes)\n",
           divergence_pct, divergence_threshold, warp_size);
    printf("# Work per branch: %d iterations\n", work_per_branch);
    printf("# Total ops: %.2e\n", total_ops);
    printf("# Effective throughput: %.2f Gops/sec\n", gops);

    // Cleanup
    CUDA_CHECK(cudaFree(d_data));
    free(h_data);

    return 0;
}
