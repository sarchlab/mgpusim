#include <hip/hip_runtime.h>

#define INF 0x7FFFFFFF

// SSSP relaxation kernel: each thread processes one node,
// relaxes all its outgoing edges using atomicMin.
extern "C" __global__ void sssp_relax_kernel(
    int *row_offsets,
    int *col_indices,
    int *edge_weights,
    int *dist,
    int *updated,
    int num_nodes)
{
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
