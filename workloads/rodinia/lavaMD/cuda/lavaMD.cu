// lavaMD.cu -- CUDA benchmark for Rodinia LavaMD
// Molecular dynamics: compute potential energy and forces between particles
// in neighboring boxes using shared memory tiling.

#include "bench_common_cuda.h"

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
    int bx = blockIdx.x;
    int tx = threadIdx.x;

    // Shared memory for neighbor box particles
    extern __shared__ char shared_mem[];
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

// Build neighbor list: 3D grid of boxes, each has up to 27 neighbors (self+26)
void build_neighbors(int boxes_per_dim, int total_boxes,
                     int *h_box_neighbors, int *h_box_num_neighbors) {
    int max_neighbors = 27;
    for (int z = 0; z < boxes_per_dim; z++) {
        for (int y = 0; y < boxes_per_dim; y++) {
            for (int x = 0; x < boxes_per_dim; x++) {
                int box_id = z * boxes_per_dim * boxes_per_dim +
                             y * boxes_per_dim + x;
                int count = 0;
                for (int dz = -1; dz <= 1; dz++) {
                    for (int dy = -1; dy <= 1; dy++) {
                        for (int dx = -1; dx <= 1; dx++) {
                            int nx = x + dx;
                            int ny = y + dy;
                            int nz = z + dz;
                            if (nx >= 0 && nx < boxes_per_dim &&
                                ny >= 0 && ny < boxes_per_dim &&
                                nz >= 0 && nz < boxes_per_dim) {
                                int neighbor_id =
                                    nz * boxes_per_dim * boxes_per_dim +
                                    ny * boxes_per_dim + nx;
                                h_box_neighbors[box_id * max_neighbors +
                                                count] = neighbor_id;
                                count++;
                            }
                        }
                    }
                }
                h_box_num_neighbors[box_id] = count;
            }
        }
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int boxes_per_dim = parseIntParam(argc, argv, "--num_boxes", 10);
    int par_per_box = parseIntParam(argc, argv, "--particles_per_box",
                                    NUMBER_PAR_PER_BOX);
    int block_size = parseIntParam(argc, argv, "--block_size", 128);

    // Parse cutoff_radius as float (use parseIntParam trick: multiply by 10)
    float cutoff_radius = 10.0f;
    for (int i = 1; i < argc - 1; i++) {
        if (strcmp(argv[i], "--cutoff_radius") == 0) {
            cutoff_radius = atof(argv[i + 1]);
            break;
        }
    }
    float cutoff2 = cutoff_radius * cutoff_radius;

    int total_boxes = boxes_per_dim * boxes_per_dim * boxes_per_dim;
    int total_particles = total_boxes * par_per_box;
    int max_neighbors = 27;

    // Generate particle data
    FOUR_VECTOR *h_rv = (FOUR_VECTOR *)malloc(total_particles *
                                               sizeof(FOUR_VECTOR));
    float *h_qv = (float *)malloc(total_particles * sizeof(float));
    FOUR_VECTOR *h_fv = (FOUR_VECTOR *)malloc(total_particles *
                                               sizeof(FOUR_VECTOR));

    srand(42);
    for (int i = 0; i < total_boxes; i++) {
        // Box center
        int bz = i / (boxes_per_dim * boxes_per_dim);
        int by = (i / boxes_per_dim) % boxes_per_dim;
        int bx = i % boxes_per_dim;
        float cx = bx * cutoff_radius;
        float cy = by * cutoff_radius;
        float cz = bz * cutoff_radius;

        for (int j = 0; j < par_per_box; j++) {
            int idx = i * par_per_box + j;
            h_rv[idx].x = cx + (rand() / (float)RAND_MAX - 0.5f) *
                          cutoff_radius;
            h_rv[idx].y = cy + (rand() / (float)RAND_MAX - 0.5f) *
                          cutoff_radius;
            h_rv[idx].z = cz + (rand() / (float)RAND_MAX - 0.5f) *
                          cutoff_radius;
            h_rv[idx].v = 0.0f;
            h_qv[idx] = (rand() / (float)RAND_MAX) * 2.0f - 1.0f;
        }
    }

    // Build neighbor list
    int *h_box_neighbors = (int *)malloc(total_boxes * max_neighbors *
                                          sizeof(int));
    int *h_box_num_neighbors = (int *)malloc(total_boxes * sizeof(int));
    build_neighbors(boxes_per_dim, total_boxes, h_box_neighbors,
                    h_box_num_neighbors);

    // Device allocations
    FOUR_VECTOR *d_rv, *d_fv;
    float *d_qv;
    int *d_box_neighbors, *d_box_num_neighbors;

    CUDA_CHECK(cudaMalloc(&d_rv, total_particles * sizeof(FOUR_VECTOR)));
    CUDA_CHECK(cudaMalloc(&d_fv, total_particles * sizeof(FOUR_VECTOR)));
    CUDA_CHECK(cudaMalloc(&d_qv, total_particles * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_box_neighbors,
                          total_boxes * max_neighbors * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_box_num_neighbors,
                          total_boxes * sizeof(int)));

    // Copy data to device
    CUDA_CHECK(cudaMemcpy(d_rv, h_rv,
                          total_particles * sizeof(FOUR_VECTOR),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_qv, h_qv, total_particles * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_box_neighbors, h_box_neighbors,
                          total_boxes * max_neighbors * sizeof(int),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_box_num_neighbors, h_box_num_neighbors,
                          total_boxes * sizeof(int),
                          cudaMemcpyHostToDevice));

    // Shared memory size
    int shmem_size = par_per_box * sizeof(FOUR_VECTOR) +
                     par_per_box * sizeof(float);

    dim3 grid(total_boxes);
    dim3 block(block_size);

    // Ensure block_size >= par_per_box
    if (block_size < par_per_box) {
        block.x = par_per_box;
    }

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", boxes_per_dim);

    printCSVHeader();

    BenchResult r = runBenchmark("lavaMD", problemSize, iterations, [&]() {
        CUDA_CHECK(cudaMemset(d_fv, 0,
                              total_particles * sizeof(FOUR_VECTOR)));
        lavaMD_kernel<<<grid, block, shmem_size>>>(
            d_rv, d_qv, d_fv, d_box_neighbors, d_box_num_neighbors,
            par_per_box, total_boxes, cutoff2);
        CUDA_CHECK(cudaDeviceSynchronize());
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_rv));
    CUDA_CHECK(cudaFree(d_fv));
    CUDA_CHECK(cudaFree(d_qv));
    CUDA_CHECK(cudaFree(d_box_neighbors));
    CUDA_CHECK(cudaFree(d_box_num_neighbors));
    free(h_rv);
    free(h_qv);
    free(h_fv);
    free(h_box_neighbors);
    free(h_box_num_neighbors);

    return 0;
}
