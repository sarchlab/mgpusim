// kmeans.cpp — HIP benchmark for K-means clustering (assign kernel)
// Kernel copied from amd/benchmarks/heteromark/kmeans/native/kmeans.cpp
// Problem size: points=1024, features=32, clusters=5

#include "bench_common.h"

#ifndef FLT_MAX
#define FLT_MAX 3.40282347e+38
#endif

__global__ void kmeans_kernel_compute(float *feature,
                                      float *clusters,
                                      int *membership, int npoints,
                                      int nclusters, int nfeatures, int offset,
                                      int size) {
    unsigned int point_id = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    int index = 0;
    if (point_id < npoints) {
        float min_dist = FLT_MAX;
        for (int i = 0; i < nclusters; i++) {
            float dist = 0;
            float ans = 0;
            for (int l = 0; l < nfeatures; l++) {
                ans += (feature[l * npoints + point_id] - clusters[i * nfeatures + l]) *
                       (feature[l * npoints + point_id] - clusters[i * nfeatures + l]);
            }

            dist = ans;
            if (dist < min_dist) {
                min_dist = dist;
                index = i;
            }
        }
        membership[point_id] = index;
    }
}

__global__ void kmeans_kernel_swap(float *feature,
                                   float *feature_swap, int npoints,
                                   int nfeatures) {
    unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    if (tid >= npoints) return;

    for (int i = 0; i < nfeatures; i++)
        feature_swap[i * npoints + tid] = feature[tid * nfeatures + i];
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int NPOINTS   = parseIntParam(argc, argv, "--points",   1024);
    int NFEATURES = parseIntParam(argc, argv, "--features", 32);
    int NCLUSTERS = parseIntParam(argc, argv, "--clusters", 5);

    // Host allocations
    float* h_feature = (float*)malloc(NPOINTS * NFEATURES * sizeof(float));
    float* h_clusters = (float*)malloc(NCLUSTERS * NFEATURES * sizeof(float));

    srand(42);
    for (int i = 0; i < NPOINTS * NFEATURES; i++)
        h_feature[i] = (float)(rand() % 1000) / 10.0f;
    for (int i = 0; i < NCLUSTERS * NFEATURES; i++)
        h_clusters[i] = (float)(rand() % 1000) / 10.0f;

    // Device allocations
    float *d_feature, *d_feature_swap, *d_clusters;
    int *d_membership;
    HIP_CHECK(hipMalloc(&d_feature, NPOINTS * NFEATURES * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_feature_swap, NPOINTS * NFEATURES * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_clusters, NCLUSTERS * NFEATURES * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_membership, NPOINTS * sizeof(int)));

    HIP_CHECK(hipMemcpy(d_feature, h_feature, NPOINTS * NFEATURES * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_clusters, h_clusters, NCLUSTERS * NFEATURES * sizeof(float), hipMemcpyHostToDevice));

    // Transpose features for coalesced access
    const int THREADS = 256;
    dim3 swapBlock(THREADS);
    dim3 swapGrid((NPOINTS + THREADS - 1) / THREADS);
    kmeans_kernel_swap<<<swapGrid, swapBlock>>>(d_feature, d_feature_swap, NPOINTS, NFEATURES);
    HIP_CHECK(hipDeviceSynchronize());

    // Benchmark the compute (assign) kernel
    dim3 block(THREADS);
    dim3 grid((NPOINTS + THREADS - 1) / THREADS);

    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "pts%d_feat%d_clus%d", NPOINTS, NFEATURES, NCLUSTERS);

    BenchResult r = runBenchmark("kmeans", problemSize, iterations, [&]() {
        kmeans_kernel_compute<<<grid, block>>>(d_feature_swap, d_clusters,
                                               d_membership, NPOINTS,
                                               NCLUSTERS, NFEATURES, 0, NPOINTS);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_feature));
    HIP_CHECK(hipFree(d_feature_swap));
    HIP_CHECK(hipFree(d_clusters));
    HIP_CHECK(hipFree(d_membership));
    free(h_feature);
    free(h_clusters);

    return 0;
}
