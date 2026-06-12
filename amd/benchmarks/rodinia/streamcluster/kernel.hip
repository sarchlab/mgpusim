#include <hip/hip_runtime.h>

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
