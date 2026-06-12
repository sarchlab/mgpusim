// sssp.cu -- CUDA benchmark for Lonestar Single-Source Shortest Path
// Bellman-Ford style iterative edge relaxation on a random weighted graph
// stored in CSR format. Uses atomicMin for distance updates.
// Iterates until no more updates occur (convergence).

#include "bench_common_cuda.h"

#define INF 0x7FFFFFFF

// SSSP relaxation kernel: each thread processes one node,
// relaxes all its outgoing edges using atomicMin.
__global__ void sssp_relax_kernel(int *row_offsets, int *col_indices,
                                  int *edge_weights, int *dist,
                                  int *updated, int num_nodes) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid < num_nodes && dist[tid] != INF) {
        int d = dist[tid];
        int row_start = row_offsets[tid];
        int row_end = row_offsets[tid + 1];
        for (int e = row_start; e < row_end; e++) {
            int neighbor = col_indices[e];
            int new_dist = d + edge_weights[e];
            int old = atomicMin(&dist[neighbor], new_dist);
            if (new_dist < old) {
                *updated = 1;
            }
        }
    }
}

// Generate a random weighted graph in CSR format programmatically
void generate_random_weighted_graph(int num_nodes, int edges_per_node,
                                    int max_weight,
                                    int **h_row_offsets, int **h_col_indices,
                                    int **h_edge_weights, int *num_edges) {
    *num_edges = num_nodes * edges_per_node;
    *h_row_offsets = (int *)malloc((num_nodes + 1) * sizeof(int));
    *h_col_indices = (int *)malloc((*num_edges) * sizeof(int));
    *h_edge_weights = (int *)malloc((*num_edges) * sizeof(int));

    srand(42);
    int offset = 0;
    for (int i = 0; i < num_nodes; i++) {
        (*h_row_offsets)[i] = offset;
        for (int j = 0; j < edges_per_node; j++) {
            (*h_col_indices)[offset + j] = rand() % num_nodes;
            (*h_edge_weights)[offset + j] = (rand() % max_weight) + 1;
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
    int max_weight = parseIntParam(argc, argv, "--max_weight", 100);

    // Generate random weighted graph
    int *h_row_offsets, *h_col_indices, *h_edge_weights;
    int num_edges;
    generate_random_weighted_graph(num_nodes, edges_per_node, max_weight,
                                   &h_row_offsets, &h_col_indices,
                                   &h_edge_weights, &num_edges);

    // Device allocations
    int *d_row_offsets, *d_col_indices, *d_edge_weights, *d_dist, *d_updated;
    CUDA_CHECK(cudaMalloc(&d_row_offsets, (num_nodes + 1) * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_col_indices, num_edges * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_edge_weights, num_edges * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_dist, num_nodes * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_updated, sizeof(int)));

    // Copy graph structure to device (constant across iterations)
    CUDA_CHECK(cudaMemcpy(d_row_offsets, h_row_offsets,
                          (num_nodes + 1) * sizeof(int),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_col_indices, h_col_indices,
                          num_edges * sizeof(int),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_edge_weights, h_edge_weights,
                          num_edges * sizeof(int),
                          cudaMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((num_nodes + block_size - 1) / block_size);

    // Host dist array for initialization
    int *h_dist = (int *)malloc(num_nodes * sizeof(int));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_nodes);

    printCSVHeader();

    BenchResult r = runBenchmark("sssp", problemSize, iterations, [&]() {
        // Initialize distances: INF for all, 0 for source
        for (int i = 0; i < num_nodes; i++) {
            h_dist[i] = INF;
        }
        h_dist[0] = 0;

        CUDA_CHECK(cudaMemcpy(d_dist, h_dist, num_nodes * sizeof(int),
                              cudaMemcpyHostToDevice));

        int h_updated = 1;
        while (h_updated) {
            h_updated = 0;
            CUDA_CHECK(cudaMemcpy(d_updated, &h_updated, sizeof(int),
                                  cudaMemcpyHostToDevice));

            sssp_relax_kernel<<<grid, block>>>(
                d_row_offsets, d_col_indices, d_edge_weights,
                d_dist, d_updated, num_nodes);
            CUDA_CHECK(cudaDeviceSynchronize());

            CUDA_CHECK(cudaMemcpy(&h_updated, d_updated, sizeof(int),
                                  cudaMemcpyDeviceToHost));
        }
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_row_offsets));
    CUDA_CHECK(cudaFree(d_col_indices));
    CUDA_CHECK(cudaFree(d_edge_weights));
    CUDA_CHECK(cudaFree(d_dist));
    CUDA_CHECK(cudaFree(d_updated));
    free(h_row_offsets);
    free(h_col_indices);
    free(h_edge_weights);
    free(h_dist);

    return 0;
}
