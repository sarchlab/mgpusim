#include <hip/hip_runtime.h>

// Triangle structure: 3 vertex indices + active flag
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

    // Clamp
    if (cos_a < -1.0f) cos_a = -1.0f;
    if (cos_a > 1.0f) cos_a = 1.0f;
    if (cos_b < -1.0f) cos_b = -1.0f;
    if (cos_b > 1.0f) cos_b = 1.0f;
    if (cos_c < -1.0f) cos_c = -1.0f;
    if (cos_c > 1.0f) cos_c = 1.0f;

    float a0 = acosf(cos_a) * 57.29577951f;
    float a1 = acosf(cos_b) * 57.29577951f;
    float a2 = acosf(cos_c) * 57.29577951f;

    float m = a0;
    if (a1 < m) m = a1;
    if (a2 < m) m = a2;
    return m;
}

// Kernel: identify bad triangles and mark them
extern "C" __global__ void identify_bad_triangles(
    Triangle *triangles, int num_triangles,
    float *px, float *py,
    int *is_bad, float min_angle_threshold,
    int *num_bad)
{
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
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
extern "C" __global__ void refine_triangles(
    Triangle *triangles, int num_triangles,
    float *px, float *py,
    int *is_bad, int *tri_locks,
    int *point_count, int *tri_count,
    int max_points, int max_triangles)
{
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid >= num_triangles) return;

    if (!is_bad[tid] || !triangles[tid].active) return;

    // Try to lock this triangle
    unsigned int old = atomicCAS((unsigned int *)&tri_locks[tid], 0u, 1u);
    if (old != 0u) return;

    Triangle t = triangles[tid];

    float x0 = px[t.v0], y0 = py[t.v0];
    float x1 = px[t.v1], y1 = py[t.v1];
    float x2 = px[t.v2], y2 = py[t.v2];

    // Compute circumcenter
    float D = 2.0f * (x0 * (y1 - y2) + x1 * (y2 - y0) + x2 * (y0 - y1));
    float cx, cy;
    if (D > -1e-10f && D < 1e-10f) {
        cx = (x0 + x1 + x2) / 3.0f;
        cy = (y0 + y1 + y2) / 3.0f;
    } else {
        cx = ((x0*x0+y0*y0)*(y1-y2) + (x1*x1+y1*y1)*(y2-y0) + (x2*x2+y2*y2)*(y0-y1)) / D;
        cy = ((x0*x0+y0*y0)*(x2-x1) + (x1*x1+y1*y1)*(x0-x2) + (x2*x2+y2*y2)*(x1-x0)) / D;
    }

    // Clamp to domain
    if (cx < 0.01f) cx = 0.01f;
    if (cx > 99.99f) cx = 99.99f;
    if (cy < 0.01f) cy = 0.01f;
    if (cy > 99.99f) cy = 99.99f;

    // Allocate new point
    int new_pt = atomicAdd(point_count, 1);
    if (new_pt >= max_points) return;

    px[new_pt] = cx;
    py[new_pt] = cy;

    // Deactivate the bad triangle
    triangles[tid].active = 0;

    // Create 3 new triangles
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
