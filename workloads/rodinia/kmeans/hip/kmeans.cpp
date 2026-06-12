// kmeans.cpp -- HIP benchmark for Rodinia K-Means Clustering
// Iteratively assigns points to nearest cluster center (GPU), then updates
// centers on CPU. Repeats until convergence or max_iterations reached.

#include "bench_common_hip.h"

// Kernel: each thread processes one point, computes distance to all centers,
// and assigns the point to the nearest cluster.
__global__ void kmeans_assign(const float *points, const float *centers,
                              int *assignments, int num_points,
                              int num_clusters, int num_features) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= num_points) return;

    float min_dist = 1e30f;
    int best = 0;

    for (int c = 0; c < num_clusters; c++) {
        float dist = 0.0f;
        for (int f = 0; f < num_features; f++) {
            float diff = points[tid * num_features + f] -
                         centers[c * num_features + f];
            dist += diff * diff;
        }
        if (dist < min_dist) {
            min_dist = dist;
            best = c;
        }
    }

    assignments[tid] = best;
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_points = parseIntParam(argc, argv, "--num_points", 65536);
    int num_clusters = parseIntParam(argc, argv, "--num_clusters", 16);
    int num_features = parseIntParam(argc, argv, "--num_features", 8);
    int max_iterations = parseIntParam(argc, argv, "--max_iterations", 20);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);

    // Generate random feature vectors programmatically
    float *h_points = (float *)malloc(num_points * num_features * sizeof(float));
    float *h_centers =
        (float *)malloc(num_clusters * num_features * sizeof(float));
    int *h_assignments = (int *)malloc(num_points * sizeof(int));

    srand(42);
    for (int i = 0; i < num_points * num_features; i++) {
        h_points[i] = (float)(rand() % 10000) / 100.0f;
    }

    // Device allocations
    float *d_points, *d_centers;
    int *d_assignments;
    HIP_CHECK(
        hipMalloc(&d_points, num_points * num_features * sizeof(float)));
    HIP_CHECK(
        hipMalloc(&d_centers, num_clusters * num_features * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_assignments, num_points * sizeof(int)));

    // Copy points to device (constant across benchmark iterations)
    HIP_CHECK(hipMemcpy(d_points, h_points,
                        num_points * num_features * sizeof(float),
                        hipMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((num_points + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_points);

    printCSVHeader();

    BenchResult r = runBenchmark("kmeans", problemSize, iterations, [&]() {
        // Initialize centers to first num_clusters points
        for (int c = 0; c < num_clusters; c++) {
            for (int f = 0; f < num_features; f++) {
                h_centers[c * num_features + f] =
                    h_points[c * num_features + f];
            }
        }

        for (int iter = 0; iter < max_iterations; iter++) {
            // Upload centers
            HIP_CHECK(hipMemcpy(d_centers, h_centers,
                                num_clusters * num_features * sizeof(float),
                                hipMemcpyHostToDevice));

            // Assign points to nearest center
            kmeans_assign<<<grid, block>>>(d_points, d_centers, d_assignments,
                                           num_points, num_clusters,
                                           num_features);
            HIP_CHECK(hipDeviceSynchronize());

            // Download assignments
            HIP_CHECK(hipMemcpy(h_assignments, d_assignments,
                                num_points * sizeof(int),
                                hipMemcpyDeviceToHost));

            // Update centers on CPU
            float *new_centers = (float *)calloc(
                num_clusters * num_features, sizeof(float));
            int *counts = (int *)calloc(num_clusters, sizeof(int));

            for (int i = 0; i < num_points; i++) {
                int c = h_assignments[i];
                counts[c]++;
                for (int f = 0; f < num_features; f++) {
                    new_centers[c * num_features + f] +=
                        h_points[i * num_features + f];
                }
            }

            int converged = 1;
            for (int c = 0; c < num_clusters; c++) {
                if (counts[c] > 0) {
                    for (int f = 0; f < num_features; f++) {
                        float new_val =
                            new_centers[c * num_features + f] / counts[c];
                        if (fabsf(new_val - h_centers[c * num_features + f]) >
                            1e-4f) {
                            converged = 0;
                        }
                        h_centers[c * num_features + f] = new_val;
                    }
                }
            }

            free(new_centers);
            free(counts);

            if (converged) break;
        }
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_points));
    HIP_CHECK(hipFree(d_centers));
    HIP_CHECK(hipFree(d_assignments));
    free(h_points);
    free(h_centers);
    free(h_assignments);

    return 0;
}
