// mst.cu -- CUDA benchmark for Lonestar Minimum Spanning Tree
// Borůvka's algorithm on GPU: iteratively (1) find the minimum-weight outgoing
// edge for each component, (2) merge components via union-find, until only
// one component remains. Random weighted undirected graph in CSR format.

#include "bench_common_cuda.h"

#define INF 0x7FFFFFFF

// ---- Union-Find helpers (device) ----

// Find root with path compression (iterative)
__device__ int find_root(int *parent, int x) {
    while (parent[x] != x) {
        parent[x] = parent[parent[x]]; // path halving
        x = parent[x];
    }
    return x;
}

// Kernel 1: For each node, find the minimum-weight edge going to a different
// component.  Store the result in min_edge_weight[comp] and min_edge_id[comp]
// using atomics (pack weight+edge_id into a long long for atomic compare).
__global__ void find_min_edge_kernel(int *row_offsets, int *col_indices,
                                     int *edge_weights, int *parent,
                                     long long *min_edge, int num_nodes) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid < num_nodes) {
        int comp = find_root(parent, tid);
        int row_start = row_offsets[tid];
        int row_end = row_offsets[tid + 1];
        for (int e = row_start; e < row_end; e++) {
            int neighbor = col_indices[e];
            int neighbor_comp = find_root(parent, neighbor);
            if (comp != neighbor_comp) {
                // Pack weight (high 32 bits) and edge index (low 32 bits)
                long long val = ((long long)edge_weights[e] << 32) | (long long)e;
                atomicMin(&min_edge[comp], val);
            }
        }
    }
}

// Kernel 2: Merge components based on minimum edges found.
// Each component with a valid min edge merges with the other endpoint's component.
__global__ void merge_components_kernel(int *col_indices, int *parent,
                                        long long *min_edge, int *changed,
                                        int num_nodes) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid < num_nodes) {
        int comp = find_root(parent, tid);
        if (comp != tid) return; // Only roots process

        long long me = min_edge[comp];
        if (me == LLONG_MAX) return; // No outgoing edge

        int edge_id = (int)(me & 0xFFFFFFFF);
        int neighbor = col_indices[edge_id];
        int neighbor_comp = find_root(parent, neighbor);

        if (comp != neighbor_comp) {
            // Merge: smaller index becomes parent (to avoid cycles)
            if (comp < neighbor_comp) {
                atomicMin(&parent[neighbor_comp], comp);
            } else {
                atomicMin(&parent[comp], neighbor_comp);
            }
            *changed = 1;
        }
    }
}

// Kernel 3: Flatten parent pointers (path compression pass)
__global__ void flatten_kernel(int *parent, int num_nodes) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid < num_nodes) {
        parent[tid] = find_root(parent, tid);
    }
}

// Generate a random weighted undirected graph in CSR format programmatically
// For simplicity, we generate directed edges; the algorithm still works on
// directed representation of undirected graphs since each undirected edge
// appears as two directed edges.
void generate_random_weighted_graph(int num_nodes, int edges_per_node,
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
            int neighbor = rand() % num_nodes;
            // Avoid self-loops
            if (neighbor == i) neighbor = (i + 1) % num_nodes;
            (*h_col_indices)[offset + j] = neighbor;
            (*h_edge_weights)[offset + j] = (rand() % 1000) + 1;
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

    // Generate random weighted graph
    int *h_row_offsets, *h_col_indices, *h_edge_weights;
    int num_edges;
    generate_random_weighted_graph(num_nodes, edges_per_node,
                                   &h_row_offsets, &h_col_indices,
                                   &h_edge_weights, &num_edges);

    // Host parent array for initialization
    int *h_parent = (int *)malloc(num_nodes * sizeof(int));

    // Device allocations
    int *d_row_offsets, *d_col_indices, *d_edge_weights, *d_parent, *d_changed;
    long long *d_min_edge;
    CUDA_CHECK(cudaMalloc(&d_row_offsets, (num_nodes + 1) * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_col_indices, num_edges * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_edge_weights, num_edges * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_parent, num_nodes * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_min_edge, num_nodes * sizeof(long long)));
    CUDA_CHECK(cudaMalloc(&d_changed, sizeof(int)));

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

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_nodes);

    printCSVHeader();

    BenchResult r = runBenchmark("mst", problemSize, iterations, [&]() {
        // Initialize parent: each node is its own component
        for (int i = 0; i < num_nodes; i++) {
            h_parent[i] = i;
        }
        CUDA_CHECK(cudaMemcpy(d_parent, h_parent, num_nodes * sizeof(int),
                              cudaMemcpyHostToDevice));

        int h_changed = 1;
        while (h_changed) {
            // Reset min_edge to LLONG_MAX
            CUDA_CHECK(cudaMemset(d_min_edge, 0x7F,
                                  num_nodes * sizeof(long long)));

            // Step 1: Find minimum outgoing edge per component
            find_min_edge_kernel<<<grid, block>>>(
                d_row_offsets, d_col_indices, d_edge_weights,
                d_parent, d_min_edge, num_nodes);
            CUDA_CHECK(cudaDeviceSynchronize());

            // Step 2: Merge components
            h_changed = 0;
            CUDA_CHECK(cudaMemcpy(d_changed, &h_changed, sizeof(int),
                                  cudaMemcpyHostToDevice));

            merge_components_kernel<<<grid, block>>>(
                d_col_indices, d_parent, d_min_edge, d_changed, num_nodes);
            CUDA_CHECK(cudaDeviceSynchronize());

            // Step 3: Flatten parent pointers
            flatten_kernel<<<grid, block>>>(d_parent, num_nodes);
            CUDA_CHECK(cudaDeviceSynchronize());

            CUDA_CHECK(cudaMemcpy(&h_changed, d_changed, sizeof(int),
                                  cudaMemcpyDeviceToHost));
        }
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_row_offsets));
    CUDA_CHECK(cudaFree(d_col_indices));
    CUDA_CHECK(cudaFree(d_edge_weights));
    CUDA_CHECK(cudaFree(d_parent));
    CUDA_CHECK(cudaFree(d_min_edge));
    CUDA_CHECK(cudaFree(d_changed));
    free(h_row_offsets);
    free(h_col_indices);
    free(h_edge_weights);
    free(h_parent);

    return 0;
}
