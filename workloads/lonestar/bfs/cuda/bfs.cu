// bfs.cu -- CUDA benchmark for Lonestar Breadth-First Search
// Data-driven worklist-based BFS on a random graph in CSR format.
// Unlike Rodinia's topology-driven BFS (which scans ALL nodes each level),
// this uses an explicit worklist (frontier) with atomic append -- only
// nodes discovered in the previous iteration are processed.

#include "bench_common_cuda.h"

// BFS worklist kernel: each thread processes one entry from the input worklist,
// discovers unvisited neighbors, and atomically appends them to the output worklist.
__global__ void bfs_worklist_kernel(int *row_offsets, int *col_indices,
                                    int *dist, int *visited,
                                    int *in_worklist, int in_size,
                                    int *out_worklist, int *out_size,
                                    int current_dist) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid < in_size) {
        int node = in_worklist[tid];
        int row_start = row_offsets[node];
        int row_end = row_offsets[node + 1];
        for (int e = row_start; e < row_end; e++) {
            int neighbor = col_indices[e];
            // Atomically try to mark as visited
            int old = atomicCAS(&visited[neighbor], 0, 1);
            if (old == 0) {
                // First time visiting this neighbor
                dist[neighbor] = current_dist;
                int pos = atomicAdd(out_size, 1);
                out_worklist[pos] = neighbor;
            }
        }
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

    // Device allocations
    int *d_row_offsets, *d_col_indices, *d_dist, *d_visited;
    int *d_worklist_a, *d_worklist_b, *d_wl_size;
    CUDA_CHECK(cudaMalloc(&d_row_offsets, (num_nodes + 1) * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_col_indices, num_edges * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_dist, num_nodes * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_visited, num_nodes * sizeof(int)));
    // Worklists -- each can hold up to num_nodes entries
    CUDA_CHECK(cudaMalloc(&d_worklist_a, num_nodes * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_worklist_b, num_nodes * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_wl_size, sizeof(int)));

    // Copy graph structure to device (constant across iterations)
    CUDA_CHECK(cudaMemcpy(d_row_offsets, h_row_offsets,
                          (num_nodes + 1) * sizeof(int),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_col_indices, h_col_indices,
                          num_edges * sizeof(int),
                          cudaMemcpyHostToDevice));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_nodes);

    printCSVHeader();

    BenchResult r = runBenchmark("bfs", problemSize, iterations, [&]() {
        // Initialize dist and visited arrays
        CUDA_CHECK(cudaMemset(d_dist, -1, num_nodes * sizeof(int)));
        CUDA_CHECK(cudaMemset(d_visited, 0, num_nodes * sizeof(int)));

        // Source node = 0
        int zero = 0;
        int one = 1;
        CUDA_CHECK(cudaMemcpy(&d_dist[0], &zero, sizeof(int),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(&d_visited[0], &one, sizeof(int),
                              cudaMemcpyHostToDevice));

        // Initialize input worklist with source node
        CUDA_CHECK(cudaMemcpy(d_worklist_a, &zero, sizeof(int),
                              cudaMemcpyHostToDevice));
        int h_in_size = 1;

        int *d_in_wl = d_worklist_a;
        int *d_out_wl = d_worklist_b;
        int current_dist = 1;

        while (h_in_size > 0) {
            // Reset output worklist size to 0
            CUDA_CHECK(cudaMemset(d_wl_size, 0, sizeof(int)));

            dim3 block(block_size);
            dim3 grid((h_in_size + block_size - 1) / block_size);

            bfs_worklist_kernel<<<grid, block>>>(
                d_row_offsets, d_col_indices, d_dist, d_visited,
                d_in_wl, h_in_size, d_out_wl, d_wl_size, current_dist);
            CUDA_CHECK(cudaDeviceSynchronize());

            // Read output worklist size
            CUDA_CHECK(cudaMemcpy(&h_in_size, d_wl_size, sizeof(int),
                                  cudaMemcpyDeviceToHost));

            // Swap worklists
            int *tmp = d_in_wl;
            d_in_wl = d_out_wl;
            d_out_wl = tmp;
            current_dist++;
        }
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_row_offsets));
    CUDA_CHECK(cudaFree(d_col_indices));
    CUDA_CHECK(cudaFree(d_dist));
    CUDA_CHECK(cudaFree(d_visited));
    CUDA_CHECK(cudaFree(d_worklist_a));
    CUDA_CHECK(cudaFree(d_worklist_b));
    CUDA_CHECK(cudaFree(d_wl_size));
    free(h_row_offsets);
    free(h_col_indices);

    return 0;
}
