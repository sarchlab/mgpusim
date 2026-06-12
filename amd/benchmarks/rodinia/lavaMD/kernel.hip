#include <hip/hip_runtime.h>

#define NUMBER_PAR_PER_BOX 100

typedef struct {
    float x, y, z, v;
} FOUR_VECTOR;

typedef struct {
    float x, y, z;
} THREE_VECTOR;

// Kernel: compute interactions between particles in a box and all
// particles in neighbor boxes, using shared memory tiling
__global__ void lavaMD_kernel(FOUR_VECTOR *d_rv, float *d_qv,
                               FOUR_VECTOR *d_fv, int *d_box_neighbors,
                               int *d_box_num_neighbors, int par_per_box,
                               int num_boxes, float cutoff2) {
    int bx = hipBlockIdx_x;
    int tx = hipThreadIdx_x;

    // Shared memory for neighbor box particles
    static __shared__ char shared_mem[65536];
    FOUR_VECTOR *sh_rv = (FOUR_VECTOR *)shared_mem;
    float *sh_qv = (float *)(sh_rv + par_per_box);

    if (bx >= num_boxes) return;

    // Load this box's particle
    int first_i = bx * par_per_box;
    FOUR_VECTOR my_rv = {0.0f, 0.0f, 0.0f, 0.0f};
    float my_qv = 0.0f;
    float fx = 0.0f, fy = 0.0f, fz = 0.0f, fv = 0.0f;

    if (tx < par_per_box) {
        my_rv = d_rv[first_i + tx];
        my_qv = d_qv[first_i + tx];
    }

    // Number of neighbors for this box
    int num_neighbors = d_box_num_neighbors[bx];
    int max_neighbors = 26 + 1; // self + 26 neighbors max

    // Iterate over all neighbor boxes (including self)
    for (int k = 0; k < num_neighbors; k++) {
        int neighbor_box = d_box_neighbors[bx * (max_neighbors) + k];
        int first_j = neighbor_box * par_per_box;

        // Load neighbor particles into shared memory
        if (tx < par_per_box) {
            sh_rv[tx] = d_rv[first_j + tx];
            sh_qv[tx] = d_qv[first_j + tx];
        }
        __syncthreads();

        // Compute interactions
        if (tx < par_per_box) {
            for (int j = 0; j < par_per_box; j++) {
                float dx = my_rv.x - sh_rv[j].x;
                float dy = my_rv.y - sh_rv[j].y;
                float dz = my_rv.z - sh_rv[j].z;
                float r2 = dx * dx + dy * dy + dz * dz;

                if (r2 > 0.0f && r2 < cutoff2) {
                    float r = sqrtf(r2);
                    float inv_r = 1.0f / r;
                    float inv_r2 = inv_r * inv_r;
                    // Lennard-Jones-like potential
                    float fs = 2.0f * my_qv * sh_qv[j] * inv_r2 * inv_r;
                    fx += fs * dx;
                    fy += fs * dy;
                    fz += fs * dz;
                    fv += my_qv * sh_qv[j] * inv_r;
                }
            }
        }
        __syncthreads();
    }

    // Write results
    if (tx < par_per_box) {
        d_fv[first_i + tx].x = fx;
        d_fv[first_i + tx].y = fy;
        d_fv[first_i + tx].z = fz;
        d_fv[first_i + tx].v = fv;
    }
}
