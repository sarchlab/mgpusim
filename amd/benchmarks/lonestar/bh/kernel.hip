#include <hip/hip_runtime.h>

#define MAX_STACK_DEPTH 64

// Tree node structure (flat array representation)
struct TreeNode {
    float cx, cy;    // center of mass
    float mass;      // total mass
    float x_min, y_min, x_max, y_max;  // bounding box
    int children[4]; // child indices (-1 = no child)
    int body_id;     // body index if leaf (-1 = internal)
};

// Force computation kernel: each thread handles one body
// Stack-based traversal of the quadtree
extern "C" __global__ void compute_forces_kernel(
    float *px, float *py, float *mass,
    float *fx, float *fy,
    TreeNode *tree, int num_bodies,
    int root, float theta, float eps2)
{
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid >= num_bodies) return;

    float bx = px[tid];
    float by = py[tid];
    float force_x = 0.0f;
    float force_y = 0.0f;

    // Stack-based traversal
    int stack[MAX_STACK_DEPTH];
    int sp = 0;
    stack[sp++] = root;

    while (sp > 0) {
        int node_idx = stack[--sp];
        if (node_idx < 0) continue;

        TreeNode node = tree[node_idx];

        // Skip if node has zero mass
        if (node.mass <= 0.0f) continue;

        float dx = node.cx - bx;
        float dy = node.cy - by;
        float dist2 = dx * dx + dy * dy + eps2;

        // Cell size (width of bounding box)
        float size = node.x_max - node.x_min;

        // If leaf node (single body)
        if (node.body_id >= 0) {
            if (node.body_id != tid) {
                float inv_dist = rsqrtf(dist2);
                float inv_dist3 = inv_dist * inv_dist * inv_dist;
                force_x += node.mass * dx * inv_dist3;
                force_y += node.mass * dy * inv_dist3;
            }
            continue;
        }

        // Barnes-Hut criterion: size/dist < theta => use center of mass
        float dist = sqrtf(dist2);
        if (size / dist < theta) {
            float inv_dist = rsqrtf(dist2);
            float inv_dist3 = inv_dist * inv_dist * inv_dist;
            force_x += node.mass * dx * inv_dist3;
            force_y += node.mass * dy * inv_dist3;
        } else {
            // Recurse into children
            for (int c = 0; c < 4; c++) {
                if (node.children[c] >= 0 && sp < MAX_STACK_DEPTH) {
                    stack[sp++] = node.children[c];
                }
            }
        }
    }

    fx[tid] = force_x;
    fy[tid] = force_y;
}

// Position/velocity update kernel
extern "C" __global__ void update_bodies_kernel(
    float *px, float *py,
    float *vx, float *vy,
    float *fx, float *fy,
    float *mass, int num_bodies,
    float dt)
{
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid >= num_bodies) return;

    float m = mass[tid];
    if (m <= 0.0f) return;

    float ax = fx[tid] / m;
    float ay = fy[tid] / m;
    vx[tid] += ax * dt;
    vy[tid] += ay * dt;
    px[tid] += vx[tid] * dt;
    py[tid] += vy[tid] * dt;
}
