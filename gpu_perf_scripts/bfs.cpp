// bfs.cpp — HIP benchmark for Breadth-First Search (warp-based)
// Kernel copied from amd/benchmarks/shoc/bfs/native/bfs.cpp
// Problem size: 1024 nodes

#include "bench_common.h"
#include <climits>
#include <queue>

__global__ void BFS_kernel_warp(
    unsigned int* levels,
    unsigned int* edgeArray,
    unsigned int* edgeArrayAux,
    int W_SZ,
    int CHUNK_SZ,
    unsigned int numVertices,
    int curr,
    int* flag)
{
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int W_OFF = tid % W_SZ;
    int W_ID = tid / W_SZ;
    int v1 = W_ID * CHUNK_SZ;
    int chk_sz = CHUNK_SZ + 1;

    if ((v1 + CHUNK_SZ) >= (int)numVertices) {
        chk_sz = (int)numVertices - v1 + 1;
        if (chk_sz < 0) chk_sz = 0;
    }

    for (int v = v1; v < chk_sz - 1 + v1; v++) {
        if (levels[v] == (unsigned int)curr) {
            unsigned int num_nbr = edgeArray[v + 1] - edgeArray[v];
            unsigned int nbr_off = edgeArray[v];
            for (int i = W_OFF; i < (int)num_nbr; i += W_SZ) {
                int nv = edgeArrayAux[i + nbr_off];
                if (levels[nv] == UINT_MAX) {
                    levels[nv] = curr + 1;
                    *flag = 1;
                }
            }
        }
    }
}

// Generate a random connected graph in CSR format
void generateGraph(int numNodes, int avgDegree,
                   std::vector<unsigned int>& rowOffset,
                   std::vector<unsigned int>& colIndex) {
    srand(42);
    rowOffset.resize(numNodes + 1, 0);

    // Build adjacency lists
    std::vector<std::vector<unsigned int>> adj(numNodes);

    // First create a spanning tree to ensure connectivity
    for (int i = 1; i < numNodes; i++) {
        int parent = rand() % i;
        adj[parent].push_back(i);
        adj[i].push_back(parent);
    }

    // Add random edges to reach desired average degree
    int targetEdges = numNodes * avgDegree / 2;
    int currentEdges = numNodes - 1;
    while (currentEdges < targetEdges) {
        int u = rand() % numNodes;
        int v = rand() % numNodes;
        if (u != v) {
            adj[u].push_back(v);
            adj[v].push_back(u);
            currentEdges++;
        }
    }

    // Convert to CSR
    colIndex.clear();
    for (int i = 0; i < numNodes; i++) {
        rowOffset[i] = (unsigned int)colIndex.size();
        for (unsigned int nb : adj[i]) {
            colIndex.push_back(nb);
        }
    }
    rowOffset[numNodes] = (unsigned int)colIndex.size();
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int NUM_NODES  = parseIntParam(argc, argv, "--nodes",  1024);
    int AVG_DEGREE = parseIntParam(argc, argv, "--degree", 6);
    const int W_SZ = 32;      // warp size for BFS kernel
    const int CHUNK_SZ = 32;  // vertices per warp

    // Generate graph
    std::vector<unsigned int> h_rowOffset;
    std::vector<unsigned int> h_colIndex;
    generateGraph(NUM_NODES, AVG_DEGREE, h_rowOffset, h_colIndex);

    int numEdges = (int)h_colIndex.size();

    // Initial levels: source node 0 at level 0, all others UINT_MAX
    std::vector<unsigned int> h_levels(NUM_NODES, UINT_MAX);
    h_levels[0] = 0;

    // Device allocations
    unsigned int *d_levels, *d_edgeArray, *d_edgeArrayAux;
    int *d_flag;
    HIP_CHECK(hipMalloc(&d_levels, NUM_NODES * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_edgeArray, (NUM_NODES + 1) * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_edgeArrayAux, numEdges * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_flag, sizeof(int)));

    HIP_CHECK(hipMemcpy(d_edgeArray, h_rowOffset.data(), (NUM_NODES + 1) * sizeof(unsigned int), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_edgeArrayAux, h_colIndex.data(), numEdges * sizeof(unsigned int), hipMemcpyHostToDevice));

    // Launch config: enough threads so each warp handles CHUNK_SZ vertices
    int numWarps = (NUM_NODES + CHUNK_SZ - 1) / CHUNK_SZ;
    int totalThreads = numWarps * W_SZ;
    const int BLOCK_SIZE = 256;
    dim3 block(BLOCK_SIZE);
    dim3 grid((totalThreads + BLOCK_SIZE - 1) / BLOCK_SIZE);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "nodes%d", NUM_NODES);

    BenchResult r = runBenchmark("bfs", problemSize, iterations, [&]() {
        // Reset levels each iteration
        HIP_CHECK(hipMemcpy(d_levels, h_levels.data(), NUM_NODES * sizeof(unsigned int), hipMemcpyHostToDevice));

        // Run BFS level by level until no more updates
        int h_flag = 1;
        int curr = 0;
        while (h_flag) {
            h_flag = 0;
            HIP_CHECK(hipMemcpy(d_flag, &h_flag, sizeof(int), hipMemcpyHostToDevice));
            BFS_kernel_warp<<<grid, block>>>(d_levels, d_edgeArray, d_edgeArrayAux,
                                              W_SZ, CHUNK_SZ, NUM_NODES, curr, d_flag);
            HIP_CHECK(hipDeviceSynchronize());
            HIP_CHECK(hipMemcpy(&h_flag, d_flag, sizeof(int), hipMemcpyDeviceToHost));
            curr++;
        }
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_levels));
    HIP_CHECK(hipFree(d_edgeArray));
    HIP_CHECK(hipFree(d_edgeArrayAux));
    HIP_CHECK(hipFree(d_flag));

    return 0;
}
