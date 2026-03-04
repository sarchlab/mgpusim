// pagerank.cpp — HIP benchmark for PageRank (SpMV-based)
// Kernel copied from amd/benchmarks/heteromark/pagerank/native/pagerank.cpp
// Problem size: nodes=64, sparsity=0.5, iterations=2

#include "bench_common.h"

__global__ void PageRankUpdateGpu(unsigned int num_rows,
                                  unsigned int *rowOffset,
                                  unsigned int *col, float *val,
                                  float *x, float *y) {
    // Shared memory for warp reduction - 64 floats per workgroup
    __shared__ float vals[64];

    int thread_id = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    int local_id = hipThreadIdx_x;
    int warp_id = thread_id / 64;
    int lane = thread_id & (64 - 1);
    int row = warp_id;

    if (row < num_rows) {
        y[row] = 0.0;
        int row_A_start = rowOffset[row];
        int row_A_end = rowOffset[row + 1];

        vals[local_id] = 0;
        for (int jj = row_A_start + lane; jj < row_A_end; jj += 64)
            vals[local_id] += val[jj] * x[col[jj]];

        // Warp reduction: 64 -> 32 -> 16 -> 8 -> 4 -> 2 -> 1
        if (lane < 32) vals[local_id] += vals[local_id + 32];
        if (lane < 16) vals[local_id] += vals[local_id + 16];
        if (lane < 8)  vals[local_id] += vals[local_id + 8];
        if (lane < 4)  vals[local_id] += vals[local_id + 4];
        if (lane < 2)  vals[local_id] += vals[local_id + 2];
        if (lane < 1)  vals[local_id] += vals[local_id + 1];
        if (lane == 0) y[row] += vals[local_id];
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    // NUM_NODES must be a multiple of 64 (WARP_SIZE used in kernel)
    int NUM_NODES  = parseIntParam(argc, argv, "--nodes", 64);
    const float SPARSITY = 0.5f;
    const int PR_ITERS   = 2;   // PageRank iterations per benchmark invocation

    // Build random sparse adjacency matrix in CSR format
    srand(42);

    // First pass: count non-zeros per row
    std::vector<unsigned int> h_rowOffset(NUM_NODES + 1, 0);
    std::vector<unsigned int> h_col;
    std::vector<float> h_val;

    for (int i = 0; i < NUM_NODES; i++) {
        int nnz_row = 0;
        for (int j = 0; j < NUM_NODES; j++) {
            float r = (float)rand() / RAND_MAX;
            if (r < SPARSITY) {
                h_col.push_back((unsigned int)j);
                nnz_row++;
            }
        }
        // Normalize values: each edge contributes 1/out_degree
        float link_val = (nnz_row > 0) ? (1.0f / nnz_row) : 0.0f;
        for (int k = 0; k < nnz_row; k++)
            h_val.push_back(link_val);
        h_rowOffset[i + 1] = h_rowOffset[i] + nnz_row;
    }

    int nnz = (int)h_col.size();

    // Initial PageRank vector: uniform
    std::vector<float> h_x(NUM_NODES, 1.0f / NUM_NODES);

    // Device allocations
    unsigned int *d_rowOffset, *d_col;
    float *d_val, *d_x, *d_y;
    HIP_CHECK(hipMalloc(&d_rowOffset, (NUM_NODES + 1) * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_col, nnz * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_val, nnz * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_x, NUM_NODES * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_y, NUM_NODES * sizeof(float)));

    HIP_CHECK(hipMemcpy(d_rowOffset, h_rowOffset.data(), (NUM_NODES + 1) * sizeof(unsigned int), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_col, h_col.data(), nnz * sizeof(unsigned int), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_val, h_val.data(), nnz * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_x, h_x.data(), NUM_NODES * sizeof(float), hipMemcpyHostToDevice));

    // Each row is handled by one warp of 64 threads
    // So we need NUM_NODES warps * 64 threads
    const int WARP_SIZE = 64;
    int totalThreads = NUM_NODES * WARP_SIZE;
    const int BLOCK_SIZE = 64;  // must equal warp size for shared memory layout
    dim3 block(BLOCK_SIZE);
    dim3 grid((totalThreads + BLOCK_SIZE - 1) / BLOCK_SIZE);

    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "nodes%d_sparsity%.1f_priters%d", NUM_NODES, SPARSITY, PR_ITERS);

    BenchResult res = runBenchmark("pagerank", problemSize, iterations, [&]() {
        // Reset x for each benchmark iteration
        HIP_CHECK(hipMemcpy(d_x, h_x.data(), NUM_NODES * sizeof(float), hipMemcpyHostToDevice));
        for (int it = 0; it < PR_ITERS; it++) {
            PageRankUpdateGpu<<<grid, block>>>(NUM_NODES, d_rowOffset, d_col, d_val, d_x, d_y);
            // Swap x and y for next iteration
            float* tmp = d_x;
            d_x = d_y;
            d_y = tmp;
        }
    });

    printCSVHeader();
    printCSVRow(res);

    // Cleanup
    HIP_CHECK(hipFree(d_rowOffset));
    HIP_CHECK(hipFree(d_col));
    HIP_CHECK(hipFree(d_val));
    HIP_CHECK(hipFree(d_x));
    HIP_CHECK(hipFree(d_y));

    return 0;
}
