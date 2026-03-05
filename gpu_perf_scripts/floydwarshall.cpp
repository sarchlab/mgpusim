// floydwarshall.cpp — HIP benchmark for Floyd-Warshall shortest path
// Kernel copied from amd/benchmarks/amdappsdk/floydwarshall/native/floydwarshall_hip.cpp
// Problem size: 256 nodes per issue #153

#include "bench_common.h"
#include <climits>

__global__ void floydWarshallPass(
    unsigned int* pathDistanceBuffer,
    unsigned int* pathBuffer,
    const unsigned int numNodes,
    const unsigned int pass)
{
    int xValue = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    int yValue = hipBlockDim_y * hipBlockIdx_y + hipThreadIdx_y;

    int k = pass;
    int oldWeight = pathDistanceBuffer[yValue * numNodes + xValue];
    int tempWeight = (pathDistanceBuffer[yValue * numNodes + k] +
                      pathDistanceBuffer[k * numNodes + xValue]);

    if (tempWeight < oldWeight) {
        pathDistanceBuffer[yValue * numNodes + xValue] = tempWeight;
        pathBuffer[yValue * numNodes + xValue] = k;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    // NUM_NODES must be a multiple of BLOCK_SIZE (8)
    unsigned int NUM_NODES = (unsigned int)parseIntParam(argc, argv, "--nodes", 256);
    unsigned int MATRIX_SIZE = NUM_NODES * NUM_NODES;
    const unsigned int BLOCK_SIZE = 8;

    // Host allocations
    unsigned int* hPathDist = (unsigned int*)malloc(MATRIX_SIZE * sizeof(unsigned int));
    unsigned int* hPathNode = (unsigned int*)malloc(MATRIX_SIZE * sizeof(unsigned int));

    // Initialize: random distances, diagonal = 0
    srand(1);
    for (unsigned int i = 0; i < NUM_NODES; i++) {
        for (unsigned int j = 0; j < NUM_NODES; j++) {
            unsigned int idx = i * NUM_NODES + j;
            if (i == j) {
                hPathDist[idx] = 0;
            } else {
                unsigned int temp = rand() % 100 + 1;
                hPathDist[idx] = temp;
            }
            hPathNode[idx] = i;
        }
    }

    // Device allocations
    unsigned int *dPathDist, *dPathNode;
    HIP_CHECK(hipMalloc(&dPathDist, MATRIX_SIZE * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&dPathNode, MATRIX_SIZE * sizeof(unsigned int)));

    // Grid/block setup
    unsigned int gridDim_val = (NUM_NODES + BLOCK_SIZE - 1) / BLOCK_SIZE;
    dim3 block(BLOCK_SIZE, BLOCK_SIZE, 1);
    dim3 grid(gridDim_val, gridDim_val, 1);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%u_nodes", NUM_NODES);

    BenchResult r = runBenchmark("floydwarshall", problemSize, iterations, [&]() {
        // Re-upload data each iteration since kernel modifies it in-place
        HIP_CHECK(hipMemcpy(dPathDist, hPathDist, MATRIX_SIZE * sizeof(unsigned int), hipMemcpyHostToDevice));
        HIP_CHECK(hipMemcpy(dPathNode, hPathNode, MATRIX_SIZE * sizeof(unsigned int), hipMemcpyHostToDevice));

        // Run all passes (k = 0 to NUM_NODES-1)
        for (unsigned int k = 0; k < NUM_NODES; k++) {
            floydWarshallPass<<<grid, block>>>(dPathDist, dPathNode, NUM_NODES, k);
        }
        HIP_CHECK(hipDeviceSynchronize());
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(dPathDist));
    HIP_CHECK(hipFree(dPathNode));
    free(hPathDist);
    free(hPathNode);

    return 0;
}
