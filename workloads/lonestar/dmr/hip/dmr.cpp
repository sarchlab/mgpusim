// dmr.cpp -- HIP benchmark for Lonestar Delaunay Mesh Refinement
// Simplified mesh refinement: generate random 2D points, build initial
// triangulation, identify bad triangles (min angle below threshold),
// and attempt to fix them by inserting circumcenters with atomic
// conflict detection. Multiple passes until convergence.

#include "bench_common_hip.h"

#define MAX_TRIANGLES_FACTOR 8
#define MAX_POINTS_FACTOR 4
#define MAX_PASSES 50

// Triangle structure: 3 vertex indices
struct Triangle {
    int v0, v1, v2;
    int active;  // 1 = active, 0 = removed
};

// Compute minimum angle of a triangle (in degrees)
__device__ float min_angle_deg(float x0, float y0, float x1, float y1,
                                float x2, float y2) {
    float ax = x1 - x0, ay = y1 - y0;
    float bx = x2 - x0, by = y2 - y0;
    float cx = x2 - x1, cy = y2 - y1;

    float la = sqrtf(ax * ax + ay * ay);
    float lb = sqrtf(bx * bx + by * by);
    float lc = sqrtf(cx * cx + cy * cy);

    if (la < 1e-10f || lb < 1e-10f || lc < 1e-10f) return 0.0f;

    // Angles via dot product
    float cos_a = (ax * bx + ay * by) / (la * lb);
    float cos_b = (-ax * cx - ay * cy) / (la * lc);
    float cos_c = (bx * cx + by * cy) / (lb * lc);

    // Clamp to avoid NaN
    cos_a = fminf(fmaxf(cos_a, -1.0f), 1.0f);
    cos_b = fminf(fmaxf(cos_b, -1.0f), 1.0f);
    cos_c = fminf(fmaxf(cos_c, -1.0f), 1.0f);

    float a0 = acosf(cos_a) * 180.0f / 3.14159265f;
    float a1 = acosf(cos_b) * 180.0f / 3.14159265f;
    float a2 = acosf(cos_c) * 180.0f / 3.14159265f;

    return fminf(a0, fminf(a1, a2));
}

// Compute circumcenter of a triangle
__device__ void circumcenter(float x0, float y0, float x1, float y1,
                              float x2, float y2, float *cx, float *cy) {
    float D = 2.0f * (x0 * (y1 - y2) + x1 * (y2 - y0) + x2 * (y0 - y1));
    if (fabsf(D) < 1e-10f) {
        *cx = (x0 + x1 + x2) / 3.0f;
        *cy = (y0 + y1 + y2) / 3.0f;
        return;
    }
    float ux = ((x0 * x0 + y0 * y0) * (y1 - y2) +
                (x1 * x1 + y1 * y1) * (y2 - y0) +
                (x2 * x2 + y2 * y2) * (y0 - y1)) / D;
    float uy = ((x0 * x0 + y0 * y0) * (x2 - x1) +
                (x1 * x1 + y1 * y1) * (x0 - x2) +
                (x2 * x2 + y2 * y2) * (x1 - x0)) / D;
    *cx = ux;
    *cy = uy;
}

// Kernel: identify bad triangles and mark them
__global__ void identify_bad_triangles(Triangle *triangles, int num_triangles,
                                        float *px, float *py,
                                        int *is_bad, float min_angle_threshold,
                                        int *num_bad) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= num_triangles) return;

    if (!triangles[tid].active) {
        is_bad[tid] = 0;
        return;
    }

    Triangle t = triangles[tid];
    float ma = min_angle_deg(px[t.v0], py[t.v0],
                              px[t.v1], py[t.v1],
                              px[t.v2], py[t.v2]);

    if (ma < min_angle_threshold) {
        is_bad[tid] = 1;
        atomicAdd(num_bad, 1);
    } else {
        is_bad[tid] = 0;
    }
}

// Kernel: attempt to refine bad triangles by inserting circumcenters
// Each bad triangle tries to claim itself via atomic CAS for conflict detection.
// If successful, it deactivates itself and creates replacement triangles.
__global__ void refine_triangles(Triangle *triangles, int num_triangles,
                                  float *px, float *py,
                                  int *is_bad, int *tri_locks,
                                  int *point_count, int *tri_count,
                                  int max_points, int max_triangles) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= num_triangles) return;

    if (!is_bad[tid] || !triangles[tid].active) return;

    // Try to lock this triangle
    int old = atomicCAS(&tri_locks[tid], 0, 1);
    if (old != 0) return;  // Another thread got it

    Triangle t = triangles[tid];

    // Compute circumcenter
    float cx, cy;
    circumcenter(px[t.v0], py[t.v0],
                 px[t.v1], py[t.v1],
                 px[t.v2], py[t.v2], &cx, &cy);

    // Clamp circumcenter to domain
    cx = fminf(fmaxf(cx, 0.01f), 99.99f);
    cy = fminf(fmaxf(cy, 0.01f), 99.99f);

    // Allocate new point
    int new_pt = atomicAdd(point_count, 1);
    if (new_pt >= max_points) return;

    px[new_pt] = cx;
    py[new_pt] = cy;

    // Deactivate the bad triangle
    triangles[tid].active = 0;

    // Create 3 new triangles from the circumcenter and original edges
    int new_tri_start = atomicAdd(tri_count, 3);
    if (new_tri_start + 2 >= max_triangles) return;

    triangles[new_tri_start].v0 = t.v0;
    triangles[new_tri_start].v1 = t.v1;
    triangles[new_tri_start].v2 = new_pt;
    triangles[new_tri_start].active = 1;

    triangles[new_tri_start + 1].v0 = t.v1;
    triangles[new_tri_start + 1].v1 = t.v2;
    triangles[new_tri_start + 1].v2 = new_pt;
    triangles[new_tri_start + 1].active = 1;

    triangles[new_tri_start + 2].v0 = t.v2;
    triangles[new_tri_start + 2].v1 = t.v0;
    triangles[new_tri_start + 2].v2 = new_pt;
    triangles[new_tri_start + 2].active = 1;
}

// Host: generate initial simple triangulation from random points
// Creates a grid-based triangulation by sorting points into cells
void generate_initial_mesh(float *px, float *py, int num_points,
                           Triangle *triangles, int *num_triangles,
                           int grid_res) {
    // Create a simple Delaunay-like triangulation using a grid approach
    // Divide domain into grid_res x grid_res cells, each cell = 2 triangles

    int tri_idx = 0;
    float cell_w = 100.0f / grid_res;
    float cell_h = 100.0f / grid_res;

    // First, create grid corner vertices as the initial points
    // Points 0..(grid_res+1)^2 - 1 are grid corners
    int grid_pts = (grid_res + 1) * (grid_res + 1);

    for (int j = 0; j <= grid_res; j++) {
        for (int i = 0; i <= grid_res; i++) {
            int idx = j * (grid_res + 1) + i;
            if (idx < num_points) {
                px[idx] = i * cell_w;
                py[idx] = j * cell_h;
            }
        }
    }

    // Add random perturbation to interior points
    srand(42);
    for (int j = 1; j < grid_res; j++) {
        for (int i = 1; i < grid_res; i++) {
            int idx = j * (grid_res + 1) + i;
            if (idx < num_points) {
                px[idx] += ((float)(rand() % 1000) / 1000.0f - 0.5f) * cell_w * 0.3f;
                py[idx] += ((float)(rand() % 1000) / 1000.0f - 0.5f) * cell_h * 0.3f;
            }
        }
    }

    // Fill remaining points with random positions
    for (int i = grid_pts; i < num_points; i++) {
        px[i] = (float)(rand() % 10000) / 10000.0f * 100.0f;
        py[i] = (float)(rand() % 10000) / 10000.0f * 100.0f;
    }

    // Create 2 triangles per grid cell
    for (int j = 0; j < grid_res; j++) {
        for (int i = 0; i < grid_res; i++) {
            int bl = j * (grid_res + 1) + i;           // bottom-left
            int br = j * (grid_res + 1) + i + 1;       // bottom-right
            int tl = (j + 1) * (grid_res + 1) + i;     // top-left
            int tr = (j + 1) * (grid_res + 1) + i + 1; // top-right

            if (bl < num_points && br < num_points &&
                tl < num_points && tr < num_points) {
                triangles[tri_idx].v0 = bl;
                triangles[tri_idx].v1 = br;
                triangles[tri_idx].v2 = tl;
                triangles[tri_idx].active = 1;
                tri_idx++;

                triangles[tri_idx].v0 = br;
                triangles[tri_idx].v1 = tr;
                triangles[tri_idx].v2 = tl;
                triangles[tri_idx].active = 1;
                tri_idx++;
            }
        }
    }

    *num_triangles = tri_idx;
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_points = parseIntParam(argc, argv, "--num_points", 16384);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int min_angle_deg_param = parseIntParam(argc, argv, "--min_angle_deg", 25);
    float min_angle_threshold = (float)min_angle_deg_param;

    // Compute grid resolution from num_points
    int grid_res = (int)sqrtf((float)num_points);
    if (grid_res < 4) grid_res = 4;

    int initial_points = (grid_res + 1) * (grid_res + 1);
    int max_points = num_points * MAX_POINTS_FACTOR;
    int max_triangles = num_points * MAX_TRIANGLES_FACTOR;

    // Host arrays
    float *h_px = (float *)malloc(max_points * sizeof(float));
    float *h_py = (float *)malloc(max_points * sizeof(float));
    Triangle *h_triangles = (Triangle *)malloc(max_triangles * sizeof(Triangle));

    // Device allocations
    float *d_px, *d_py;
    Triangle *d_triangles;
    int *d_is_bad, *d_tri_locks, *d_num_bad;
    int *d_point_count, *d_tri_count;

    HIP_CHECK(hipMalloc(&d_px, max_points * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_py, max_points * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_triangles, max_triangles * sizeof(Triangle)));
    HIP_CHECK(hipMalloc(&d_is_bad, max_triangles * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_tri_locks, max_triangles * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_num_bad, sizeof(int)));
    HIP_CHECK(hipMalloc(&d_point_count, sizeof(int)));
    HIP_CHECK(hipMalloc(&d_tri_count, sizeof(int)));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_points);

    printCSVHeader();

    BenchResult r = runBenchmark("dmr", problemSize, iterations, [&]() {
        // Generate initial mesh
        int h_num_triangles = 0;
        generate_initial_mesh(h_px, h_py, initial_points,
                              h_triangles, &h_num_triangles, grid_res);

        int h_point_count = initial_points;
        int h_tri_count = h_num_triangles;

        // Copy to device
        HIP_CHECK(hipMemcpy(d_px, h_px, h_point_count * sizeof(float),
                            hipMemcpyHostToDevice));
        HIP_CHECK(hipMemcpy(d_py, h_py, h_point_count * sizeof(float),
                            hipMemcpyHostToDevice));
        HIP_CHECK(hipMemcpy(d_triangles, h_triangles,
                            h_tri_count * sizeof(Triangle),
                            hipMemcpyHostToDevice));
        HIP_CHECK(hipMemcpy(d_point_count, &h_point_count, sizeof(int),
                            hipMemcpyHostToDevice));
        HIP_CHECK(hipMemcpy(d_tri_count, &h_tri_count, sizeof(int),
                            hipMemcpyHostToDevice));

        // Iterative refinement
        for (int pass = 0; pass < MAX_PASSES; pass++) {
            int h_num_bad = 0;
            HIP_CHECK(hipMemcpy(d_num_bad, &h_num_bad, sizeof(int),
                                hipMemcpyHostToDevice));

            // Read current triangle count
            HIP_CHECK(hipMemcpy(&h_tri_count, d_tri_count, sizeof(int),
                                hipMemcpyDeviceToHost));

            if (h_tri_count <= 0) break;

            dim3 block(block_size);
            dim3 grid((h_tri_count + block_size - 1) / block_size);

            // Phase 1: identify bad triangles
            identify_bad_triangles<<<grid, block>>>(
                d_triangles, h_tri_count, d_px, d_py,
                d_is_bad, min_angle_threshold, d_num_bad);
            HIP_CHECK(hipDeviceSynchronize());

            // Check if any bad triangles remain
            HIP_CHECK(hipMemcpy(&h_num_bad, d_num_bad, sizeof(int),
                                hipMemcpyDeviceToHost));
            if (h_num_bad == 0) break;

            // Reset locks
            HIP_CHECK(hipMemset(d_tri_locks, 0,
                                h_tri_count * sizeof(int)));

            // Phase 2: refine bad triangles
            refine_triangles<<<grid, block>>>(
                d_triangles, h_tri_count, d_px, d_py,
                d_is_bad, d_tri_locks,
                d_point_count, d_tri_count,
                max_points, max_triangles);
            HIP_CHECK(hipDeviceSynchronize());
        }
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_px));
    HIP_CHECK(hipFree(d_py));
    HIP_CHECK(hipFree(d_triangles));
    HIP_CHECK(hipFree(d_is_bad));
    HIP_CHECK(hipFree(d_tri_locks));
    HIP_CHECK(hipFree(d_num_bad));
    HIP_CHECK(hipFree(d_point_count));
    HIP_CHECK(hipFree(d_tri_count));
    free(h_px);
    free(h_py);
    free(h_triangles);

    return 0;
}
