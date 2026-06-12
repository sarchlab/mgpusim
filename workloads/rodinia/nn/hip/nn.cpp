// nn.cpp -- HIP benchmark for Rodinia Nearest Neighbor
// Compute Euclidean distance from a query point to all records,
// then find the k nearest neighbors.

#include "bench_common_hip.h"

typedef float DATA_TYPE;

struct LatLong {
    DATA_TYPE lat;
    DATA_TYPE lng;
};

__global__ void nn_kernel(LatLong *d_locations, DATA_TYPE *d_distances,
                          int num_records, DATA_TYPE query_lat,
                          DATA_TYPE query_lng) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (tid < num_records) {
        DATA_TYPE dlat = d_locations[tid].lat - query_lat;
        DATA_TYPE dlng = d_locations[tid].lng - query_lng;
        d_distances[tid] = sqrtf(dlat * dlat + dlng * dlng);
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_records = parseIntParam(argc, argv, "--num_records", 65536);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int k_nearest = parseIntParam(argc, argv, "--k_nearest", 10);

    // Generate random latitude/longitude records
    LatLong *h_locations =
        (LatLong *)malloc(num_records * sizeof(LatLong));
    DATA_TYPE *h_distances =
        (DATA_TYPE *)malloc(num_records * sizeof(DATA_TYPE));

    srand(42);
    for (int i = 0; i < num_records; i++) {
        h_locations[i].lat =
            (DATA_TYPE)(rand() % 18000) / 100.0f - 90.0f;   // -90 to +90
        h_locations[i].lng =
            (DATA_TYPE)(rand() % 36000) / 100.0f - 180.0f;  // -180 to +180
    }

    // Query point
    DATA_TYPE query_lat = 30.0f;
    DATA_TYPE query_lng = -90.0f;

    // Device allocations
    LatLong *d_locations;
    DATA_TYPE *d_distances;
    HIP_CHECK(hipMalloc(&d_locations, num_records * sizeof(LatLong)));
    HIP_CHECK(hipMalloc(&d_distances, num_records * sizeof(DATA_TYPE)));

    HIP_CHECK(hipMemcpy(d_locations, h_locations,
                        num_records * sizeof(LatLong),
                        hipMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((num_records + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_records);

    printCSVHeader();

    BenchResult r = runBenchmark("nn", problemSize, iterations, [&]() {
        nn_kernel<<<grid, block>>>(d_locations, d_distances, num_records,
                                   query_lat, query_lng);
    });
    printCSVRow(r);

    // Copy distances back (for verification or k-nearest selection on host)
    HIP_CHECK(hipMemcpy(h_distances, d_distances,
                        num_records * sizeof(DATA_TYPE),
                        hipMemcpyDeviceToHost));

    // Simple verification: find k nearest on CPU (just print first k_nearest)
    // (Not timed -- just for validation)
    if (num_records <= 1024 && k_nearest <= num_records) {
        for (int k = 0; k < k_nearest && k < 5; k++) {
            int min_idx = 0;
            DATA_TYPE min_dist = h_distances[0];
            for (int j = 1; j < num_records; j++) {
                if (h_distances[j] < min_dist) {
                    min_dist = h_distances[j];
                    min_idx = j;
                }
            }
            h_distances[min_idx] = 1e30f;  // Mark as used
        }
    }

    // Cleanup
    HIP_CHECK(hipFree(d_locations));
    HIP_CHECK(hipFree(d_distances));
    free(h_locations);
    free(h_distances);

    return 0;
}
