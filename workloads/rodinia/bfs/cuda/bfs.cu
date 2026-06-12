// bfs.cu -- CUDA benchmark for Rodinia Breadth-First Search
// Iterative BFS on a random graph stored in CSR format.
// 1 kernel launched iteratively: marks frontier nodes and updates costs.

#include "bench_common_cuda.h"

typedef int DATA_TYPE;

// BFS kernel: each thread processes one node in the frontier
__global__ void bfs_kernel(int *row_offsets, int *col_indices, int *cost,
                           int *frontier, int *visited, int *updated,
                           int num_nodes) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid < num_nodes && frontier[tid]) {
        frontier[tid] = 0;
        int row_start = row_offsets[tid];
        int row_end = row_offsets[tid + 1];
        for (int e = row_start; e < row_end; e++) {
            int neighbor = col_indices[e];
            if (!visited[neighbor]) {
                cost[neighbor] = cost[tid] + 1;
                *updated = 1;
            }
        }
    }
}

// Second pass: update visited and frontier arrays
__global__ void bfs_update(int *cost, int *frontier, int *visited,
                           int *updated, int num_nodes) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid < num_nodes && !visited[tid] && cost[tid] >= 0) {
        visited[tid] = 1;
        frontier[tid] = 1;
        *updated = 1;
    }
}

// Generate a random graph in CSR format programmatically
void generate_random_graph(int num_nodes, int edges_per_node,
                           int **h_row_offsets, int **h_col_indices,
                           int *num_edges) {
    *num_edges = num_nodes * edges_per_node;
    *h_row_offsets = (int *)malloc((num_nodes + 1) * sizeof(int));
    *h_col_indices = (int *)malloc((*num_edges) * sizeof(int));

    srand(42);
    int offset = 0;
    for (int i = 0; i < num_nodes; i++) {
        (*h_row_offsets)[i] = offset;
        for (int j = 0; j < edges_per_node; j++) {
            (*h_col_indices)[offset + j] = rand() % num_nodes;
        }
        offset += edges_per_node;
    }
    (*h_row_offsets)[num_nodes] = offset;
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_nodes = parseIntParam(argc, argv, "--num_nodes", 65536);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int edges_per_node = parseIntParam(argc, argv, "--edges_per_node", 6);

    // Generate random graph
    int *h_row_offsets, *h_col_indices;
    int num_edges;
    generate_random_graph(num_nodes, edges_per_node, &h_row_offsets,
                          &h_col_indices, &num_edges);

    // Host arrays
    int *h_cost = (int *)malloc(num_nodes * sizeof(int));
    int *h_frontier = (int *)malloc(num_nodes * sizeof(int));
    int *h_visited = (int *)malloc(num_nodes * sizeof(int));

    // Device allocations
    int *d_row_offsets, *d_col_indices, *d_cost, *d_frontier, *d_visited,
        *d_updated;
    CUDA_CHECK(
        cudaMalloc(&d_row_offsets, (num_nodes + 1) * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_col_indices, num_edges * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_cost, num_nodes * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_frontier, num_nodes * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_visited, num_nodes * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_updated, sizeof(int)));

    // Copy graph structure to device (constant across iterations)
    CUDA_CHECK(cudaMemcpy(d_row_offsets, h_row_offsets,
                          (num_nodes + 1) * sizeof(int),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_col_indices, h_col_indices,
                          num_edges * sizeof(int),
                          cudaMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((num_nodes + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_nodes);

    printCSVHeader();

    BenchResult r = runBenchmark("bfs", problemSize, iterations, [&]() {
        // Initialize BFS arrays for each run
        for (int i = 0; i < num_nodes; i++) {
            h_cost[i] = -1;
            h_frontier[i] = 0;
            h_visited[i] = 0;
        }
        // Source node = 0
        h_cost[0] = 0;
        h_frontier[0] = 1;
        h_visited[0] = 1;

        CUDA_CHECK(cudaMemcpy(d_cost, h_cost, num_nodes * sizeof(int),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(d_frontier, h_frontier,
                              num_nodes * sizeof(int),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(d_visited, h_visited,
                              num_nodes * sizeof(int),
                              cudaMemcpyHostToDevice));

        int h_updated = 1;
        while (h_updated) {
            h_updated = 0;
            CUDA_CHECK(cudaMemcpy(d_updated, &h_updated, sizeof(int),
                                  cudaMemcpyHostToDevice));

            bfs_kernel<<<grid, block>>>(d_row_offsets, d_col_indices, d_cost,
                                        d_frontier, d_visited, d_updated,
                                        num_nodes);
            CUDA_CHECK(cudaDeviceSynchronize());

            h_updated = 0;
            CUDA_CHECK(cudaMemcpy(d_updated, &h_updated, sizeof(int),
                                  cudaMemcpyHostToDevice));

            bfs_update<<<grid, block>>>(d_cost, d_frontier, d_visited,
                                        d_updated, num_nodes);
            CUDA_CHECK(cudaDeviceSynchronize());

            CUDA_CHECK(cudaMemcpy(&h_updated, d_updated, sizeof(int),
                                  cudaMemcpyDeviceToHost));
        }
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_row_offsets));
    CUDA_CHECK(cudaFree(d_col_indices));
    CUDA_CHECK(cudaFree(d_cost));
    CUDA_CHECK(cudaFree(d_frontier));
    CUDA_CHECK(cudaFree(d_visited));
    CUDA_CHECK(cudaFree(d_updated));
    free(h_row_offsets);
    free(h_col_indices);
    free(h_cost);
    free(h_frontier);
    free(h_visited);

    return 0;
}
