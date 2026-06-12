// streamcluster.cpp -- HIP benchmark for Rodinia Streamcluster
// Online clustering of streaming data points.
// 1 main GPU kernel: pgain -- computes cost change of opening a new center.

#include "bench_common_hip.h"

// Kernel: compute cost change (pgain) for opening a new center
// Each thread processes one point to compute how much the total cost changes
// if the candidate center is opened.
__global__ void pgain_kernel(float *coord, float *center_cost,
                              int *center_assign, float *work_cost,
                              int *work_assign, int candidate,
                              int num_points, int dim,
                              float *cost_change) {
    int pid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (pid >= num_points) return;

    // Compute distance from this point to candidate center
    float dist = 0.0f;
    int p_offset = pid * dim;
    int c_offset = candidate * dim;
    for (int d = 0; d < dim; d++) {
        float diff = coord[p_offset + d] - coord[c_offset + d];
        dist += diff * diff;
    }

    // Current cost for this point
    float current_cost = center_cost[pid];

    // If candidate is closer, update
    if (dist < current_cost) {
        // Cost reduction
        float reduction = current_cost - dist;
        atomicAdd(cost_change, -reduction);
        work_cost[pid] = dist;
        work_assign[pid] = candidate;
    } else {
        work_cost[pid] = current_cost;
        work_assign[pid] = center_assign[pid];
    }
}

// Host function: simple local search for clustering
void local_search(float *h_coord, int num_points, int dim,
                  int num_centers, int block_size, int iterations_bench) {
    // Allocate host arrays
    float *h_center_cost = (float *)malloc(num_points * sizeof(float));
    int *h_center_assign = (int *)malloc(num_points * sizeof(int));

    // Initialize: assign each point to center 0 (first point)
    srand(42);
    for (int i = 0; i < num_points; i++) {
        // Compute distance to center 0
        float dist = 0.0f;
        for (int d = 0; d < dim; d++) {
            float diff = h_coord[i * dim + d] - h_coord[0 * dim + d];
            dist += diff * diff;
        }
        h_center_cost[i] = dist;
        h_center_assign[i] = 0;
    }

    // Device allocations
    float *d_coord, *d_center_cost, *d_work_cost, *d_cost_change;
    int *d_center_assign, *d_work_assign;

    HIP_CHECK(hipMalloc(&d_coord, num_points * dim * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_center_cost, num_points * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_work_cost, num_points * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_center_assign, num_points * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_work_assign, num_points * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_cost_change, sizeof(float)));

    HIP_CHECK(hipMemcpy(d_coord, h_coord,
                        num_points * dim * sizeof(float),
                        hipMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((num_points + block_size - 1) / block_size);

    // Try opening each candidate center
    int centers_opened = 0;
    for (int c = 1; c < num_points && centers_opened < num_centers; c++) {
        // Pick candidate (every num_points/num_centers-th point)
        int candidate = c * (num_points / num_centers);
        if (candidate >= num_points) candidate = num_points - 1;

        HIP_CHECK(hipMemcpy(d_center_cost, h_center_cost,
                            num_points * sizeof(float),
                            hipMemcpyHostToDevice));
        HIP_CHECK(hipMemcpy(d_center_assign, h_center_assign,
                            num_points * sizeof(int),
                            hipMemcpyHostToDevice));

        float h_cost_change = 0.0f;
        HIP_CHECK(hipMemcpy(d_cost_change, &h_cost_change,
                            sizeof(float), hipMemcpyHostToDevice));

        pgain_kernel<<<grid, block>>>(
            d_coord, d_center_cost, d_center_assign,
            d_work_cost, d_work_assign, candidate,
            num_points, dim, d_cost_change);
        HIP_CHECK(hipDeviceSynchronize());

        HIP_CHECK(hipMemcpy(&h_cost_change, d_cost_change,
                            sizeof(float), hipMemcpyDeviceToHost));

        // Accept if cost decreases
        if (h_cost_change < 0.0f) {
            HIP_CHECK(hipMemcpy(h_center_cost, d_work_cost,
                                num_points * sizeof(float),
                                hipMemcpyDeviceToHost));
            HIP_CHECK(hipMemcpy(h_center_assign, d_work_assign,
                                num_points * sizeof(int),
                                hipMemcpyDeviceToHost));
            centers_opened++;
        }
    }

    // Cleanup device
    HIP_CHECK(hipFree(d_coord));
    HIP_CHECK(hipFree(d_center_cost));
    HIP_CHECK(hipFree(d_work_cost));
    HIP_CHECK(hipFree(d_center_assign));
    HIP_CHECK(hipFree(d_work_assign));
    HIP_CHECK(hipFree(d_cost_change));

    free(h_center_cost);
    free(h_center_assign);
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_points = parseIntParam(argc, argv, "--num_points", 65536);
    int num_dimensions = parseIntParam(argc, argv, "--num_dimensions", 32);
    int num_centers = parseIntParam(argc, argv, "--num_centers", 10);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);

    // Generate random point data
    float *h_coord = (float *)malloc(num_points * num_dimensions *
                                      sizeof(float));
    srand(42);
    for (int i = 0; i < num_points * num_dimensions; i++) {
        h_coord[i] = (rand() / (float)RAND_MAX) * 100.0f;
    }

    // --- Benchmark pgain kernel directly ---
    // Setup initial assignment (all points assigned to center 0)
    float *h_center_cost = (float *)malloc(num_points * sizeof(float));
    int *h_center_assign = (int *)malloc(num_points * sizeof(int));

    for (int i = 0; i < num_points; i++) {
        float dist = 0.0f;
        for (int d = 0; d < num_dimensions; d++) {
            float diff = h_coord[i * num_dimensions + d] -
                         h_coord[0 * num_dimensions + d];
            dist += diff * diff;
        }
        h_center_cost[i] = dist;
        h_center_assign[i] = 0;
    }

    // Pick a candidate center in the middle
    int candidate = num_points / 2;

    // Device allocations
    float *d_coord, *d_center_cost, *d_work_cost, *d_cost_change;
    int *d_center_assign, *d_work_assign;

    HIP_CHECK(hipMalloc(&d_coord,
                        num_points * num_dimensions * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_center_cost, num_points * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_work_cost, num_points * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_center_assign, num_points * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_work_assign, num_points * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_cost_change, sizeof(float)));

    HIP_CHECK(hipMemcpy(d_coord, h_coord,
                        num_points * num_dimensions * sizeof(float),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_center_cost, h_center_cost,
                        num_points * sizeof(float),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_center_assign, h_center_assign,
                        num_points * sizeof(int),
                        hipMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((num_points + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_points);

    printCSVHeader();

    BenchResult r = runBenchmark("streamcluster_pgain", problemSize,
                                  iterations, [&]() {
        float h_cost_change = 0.0f;
        HIP_CHECK(hipMemcpy(d_cost_change, &h_cost_change,
                            sizeof(float), hipMemcpyHostToDevice));

        pgain_kernel<<<grid, block>>>(
            d_coord, d_center_cost, d_center_assign,
            d_work_cost, d_work_assign, candidate,
            num_points, num_dimensions, d_cost_change);
        HIP_CHECK(hipDeviceSynchronize());
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_coord));
    HIP_CHECK(hipFree(d_center_cost));
    HIP_CHECK(hipFree(d_work_cost));
    HIP_CHECK(hipFree(d_center_assign));
    HIP_CHECK(hipFree(d_work_assign));
    HIP_CHECK(hipFree(d_cost_change));
    free(h_coord);
    free(h_center_cost);
    free(h_center_assign);

    return 0;
}
