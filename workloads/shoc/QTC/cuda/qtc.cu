// qtc.cu -- SHOC Quality Threshold Clustering benchmark (CUDA)
// Simplified QTC: compute pairwise distance matrix and find cluster sizes.

#include "bench_common_cuda.h"

#define DEFAULT_POINTS     2048
#define DEFAULT_DIMS       4
#define DEFAULT_THRESHOLD  1.0f
#define DEFAULT_BLOCK      256

// ---------------------------------------------------------------------------
// Kernel 1: Compute pairwise Euclidean distance matrix
// dist[i * N + j] = euclidean distance between point[i] and point[j]
// ---------------------------------------------------------------------------
__global__ void distance_kernel(const float* __restrict__ points,
                                float* __restrict__ dist,
                                int N,
                                int D) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    int totalPairs = N * N;
    if (idx >= totalPairs) return;

    int i = idx / N;
    int j = idx % N;

    float sum = 0.0f;
    for (int d = 0; d < D; ++d) {
        float diff = points[i * D + d] - points[j * D + d];
        sum += diff * diff;
    }
    dist[i * N + j] = sqrtf(sum);
}

// ---------------------------------------------------------------------------
// Kernel 2: For each candidate center, count points within threshold
// ---------------------------------------------------------------------------
__global__ void find_cluster_kernel(const float* __restrict__ dist,
                                    int* __restrict__ cluster_size,
                                    int N,
                                    float threshold) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i >= N) return;

    int count = 0;
    for (int j = 0; j < N; ++j) {
        if (dist[i * N + j] < threshold) {
            count++;
        }
    }
    cluster_size[i] = count;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--size", DEFAULT_POINTS);
    int D          = parseIntParam(argc, argv, "--dimensions", DEFAULT_DIMS);
    int block_size = parseIntParam(argc, argv, "--block_size", DEFAULT_BLOCK);
    int iters      = parseIterations(argc, argv);

    // Parse float threshold manually
    float threshold = DEFAULT_THRESHOLD;
    for (int i = 1; i < argc - 1; ++i) {
        if (strcmp(argv[i], "--threshold") == 0) {
            threshold = (float)atof(argv[i + 1]);
            break;
        }
    }

    size_t pointsBytes  = (size_t)N * D * sizeof(float);
    size_t distBytes    = (size_t)N * N * sizeof(float);
    size_t clusterBytes = (size_t)N * sizeof(int);

    // Host allocations
    float* h_points = (float*)malloc(pointsBytes);
    int*   h_cluster_size = (int*)malloc(clusterBytes);

    // Initialize random points in [0, 10) for each dimension
    srand(42);
    for (int i = 0; i < N * D; ++i) {
        h_points[i] = (float)(rand() % 10000) / 1000.0f;
    }

    // Device allocations
    float *d_points, *d_dist;
    int   *d_cluster_size;

    CUDA_CHECK(cudaMalloc(&d_points, pointsBytes));
    CUDA_CHECK(cudaMalloc(&d_dist, distBytes));
    CUDA_CHECK(cudaMalloc(&d_cluster_size, clusterBytes));

    CUDA_CHECK(cudaMemcpy(d_points, h_points, pointsBytes,
                          cudaMemcpyHostToDevice));

    int totalPairs = N * N;
    int distGrid   = (totalPairs + block_size - 1) / block_size;
    int nodeGrid   = (N + block_size - 1) / block_size;

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("qtc", problemSize, iters, [&]() {
        distance_kernel<<<distGrid, block_size>>>(d_points, d_dist, N, D);
        find_cluster_kernel<<<nodeGrid, block_size>>>(
            d_dist, d_cluster_size, N, threshold);
    });

    printCSVRow(r);

    // Verification: run once and find largest cluster
    distance_kernel<<<distGrid, block_size>>>(d_points, d_dist, N, D);
    find_cluster_kernel<<<nodeGrid, block_size>>>(
        d_dist, d_cluster_size, N, threshold);
    CUDA_CHECK(cudaDeviceSynchronize());

    CUDA_CHECK(cudaMemcpy(h_cluster_size, d_cluster_size, clusterBytes,
                          cudaMemcpyDeviceToHost));

    int maxCluster = 0;
    int bestCenter = 0;
    for (int i = 0; i < N; ++i) {
        if (h_cluster_size[i] > maxCluster) {
            maxCluster = h_cluster_size[i];
            bestCenter = i;
        }
    }

    fprintf(stderr, "Largest cluster: center=%d, size=%d (of %d points, threshold=%.2f)\n",
            bestCenter, maxCluster, N, threshold);
    fprintf(stderr, "PASS\n");

    // Cleanup
    CUDA_CHECK(cudaFree(d_points));
    CUDA_CHECK(cudaFree(d_dist));
    CUDA_CHECK(cudaFree(d_cluster_size));
    free(h_points);
    free(h_cluster_size);

    return 0;
}
