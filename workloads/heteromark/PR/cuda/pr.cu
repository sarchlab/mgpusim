// pr.cu -- Heteromark PageRank benchmark (CUDA)
// PageRank using iterative sparse matrix-vector multiply (pull-based, CSR).

#include "bench_common_cuda.h"

#define DEFAULT_NODES      4096
#define DEFAULT_AVG_DEGREE 8
#define DEFAULT_ITERATIONS 20
#define DEFAULT_BLOCK      256
#define DAMPING            0.85f

// ---------------------------------------------------------------------------
// Kernel: PageRank iteration (pull-based)
// ---------------------------------------------------------------------------
__global__ void pagerank_kernel(const int* __restrict__ row_offsets,
                                const int* __restrict__ col_indices,
                                const int* __restrict__ out_degree,
                                const float* __restrict__ rank_in,
                                float* __restrict__ rank_out,
                                int N,
                                float damping,
                                float base_rank) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i >= N) return;

    float sum = 0.0f;
    int start = row_offsets[i];
    int end   = row_offsets[i + 1];

    for (int j = start; j < end; ++j) {
        int neighbor = col_indices[j];
        sum += rank_in[neighbor] / (float)out_degree[neighbor];
    }

    rank_out[i] = base_rank + damping * sum;
}

// ---------------------------------------------------------------------------
// Generate random graph in CSR format
// ---------------------------------------------------------------------------
void generateRandomGraph(int N, int avgDegree,
                         int** h_row_offsets, int** h_col_indices,
                         int** h_out_degree, int* numEdges) {
    *h_row_offsets = (int*)malloc((N + 1) * sizeof(int));
    *h_out_degree  = (int*)malloc(N * sizeof(int));

    // First pass: determine degree for each node
    srand(42);
    int totalEdges = 0;
    for (int i = 0; i < N; ++i) {
        // Randomize degree around avgDegree (Poisson-like)
        int deg = avgDegree / 2 + (rand() % (avgDegree + 1));
        if (deg < 1) deg = 1;
        if (deg > N - 1) deg = N - 1;
        (*h_out_degree)[i] = deg;
        totalEdges += deg;
    }

    *numEdges = totalEdges;
    *h_col_indices = (int*)malloc(totalEdges * sizeof(int));

    // Build CSR row_offsets and col_indices
    (*h_row_offsets)[0] = 0;
    int offset = 0;
    for (int i = 0; i < N; ++i) {
        int deg = (*h_out_degree)[i];
        for (int j = 0; j < deg; ++j) {
            int target = rand() % N;
            // Avoid self-loops
            if (target == i) target = (i + 1) % N;
            (*h_col_indices)[offset + j] = target;
        }
        offset += deg;
        (*h_row_offsets)[i + 1] = offset;
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N             = parseIntParam(argc, argv, "--size", DEFAULT_NODES);
    int avgDegree     = parseIntParam(argc, argv, "--avg_degree", DEFAULT_AVG_DEGREE);
    int numIterations = parseIntParam(argc, argv, "--num_iterations", DEFAULT_ITERATIONS);
    int block_size    = parseIntParam(argc, argv, "--block_size", DEFAULT_BLOCK);
    int iters         = parseIterations(argc, argv);

    // Generate random graph
    int *h_row_offsets, *h_col_indices, *h_out_degree;
    int numEdges;
    generateRandomGraph(N, avgDegree, &h_row_offsets, &h_col_indices,
                        &h_out_degree, &numEdges);

    // Initialize rank
    float* h_rank = (float*)malloc(N * sizeof(float));
    float initRank = 1.0f / (float)N;
    for (int i = 0; i < N; ++i) {
        h_rank[i] = initRank;
    }

    // Device allocations
    int *d_row_offsets, *d_col_indices, *d_out_degree;
    float *d_rank, *d_rank_new;

    CUDA_CHECK(cudaMalloc(&d_row_offsets, (N + 1) * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_col_indices, numEdges * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_out_degree,  N * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_rank,        N * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_rank_new,    N * sizeof(float)));

    CUDA_CHECK(cudaMemcpy(d_row_offsets, h_row_offsets, (N + 1) * sizeof(int),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_col_indices, h_col_indices, numEdges * sizeof(int),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_out_degree, h_out_degree, N * sizeof(int),
                          cudaMemcpyHostToDevice));

    int grid = (N + block_size - 1) / block_size;
    float base_rank = (1.0f - DAMPING) / (float)N;

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("pagerank", problemSize, iters, [&]() {
        // Re-init rank each benchmark iteration
        CUDA_CHECK(cudaMemcpy(d_rank, h_rank, N * sizeof(float),
                              cudaMemcpyHostToDevice));

        for (int it = 0; it < numIterations; ++it) {
            pagerank_kernel<<<grid, block_size>>>(
                d_row_offsets, d_col_indices, d_out_degree,
                d_rank, d_rank_new, N, DAMPING, base_rank);

            // Swap rank pointers
            float* tmp = d_rank;
            d_rank     = d_rank_new;
            d_rank_new = tmp;
        }
    });

    printCSVRow(r);

    // Verification: check rank sum ≈ 1.0
    CUDA_CHECK(cudaMemcpy(d_rank, h_rank, N * sizeof(float),
                          cudaMemcpyHostToDevice));
    for (int it = 0; it < numIterations; ++it) {
        pagerank_kernel<<<grid, block_size>>>(
            d_row_offsets, d_col_indices, d_out_degree,
            d_rank, d_rank_new, N, DAMPING, base_rank);
        float* tmp = d_rank;
        d_rank     = d_rank_new;
        d_rank_new = tmp;
    }
    CUDA_CHECK(cudaDeviceSynchronize());

    float* h_result = (float*)malloc(N * sizeof(float));
    CUDA_CHECK(cudaMemcpy(h_result, d_rank, N * sizeof(float),
                          cudaMemcpyDeviceToHost));

    double rankSum = 0.0;
    for (int i = 0; i < N; ++i) {
        rankSum += h_result[i];
    }

    fprintf(stderr, "Rank sum after %d iterations: %.6f (expected ~1.0)\n",
            numIterations, rankSum);

    if (fabs(rankSum - 1.0) < 0.1) {
        fprintf(stderr, "PASS\n");
    } else {
        fprintf(stderr, "WARN: rank sum deviates from 1.0\n");
    }

    // Cleanup
    CUDA_CHECK(cudaFree(d_row_offsets));
    CUDA_CHECK(cudaFree(d_col_indices));
    CUDA_CHECK(cudaFree(d_out_degree));
    CUDA_CHECK(cudaFree(d_rank));
    CUDA_CHECK(cudaFree(d_rank_new));
    free(h_row_offsets);
    free(h_col_indices);
    free(h_out_degree);
    free(h_rank);
    free(h_result);

    return 0;
}
