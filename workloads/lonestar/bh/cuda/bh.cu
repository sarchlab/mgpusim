// bh.cu -- CUDA benchmark for Lonestar Barnes-Hut n-body simulation
// Simplified 2D quadtree-based gravitational force computation.
// Phase 1: Build quadtree (on host, flat array representation)
// Phase 2: GPU kernel computes forces via stack-based tree traversal
// Phase 3: GPU kernel updates positions and velocities

#include "bench_common_cuda.h"

// Maximum tree nodes: ~4x num_bodies is typical for a quadtree
#define MAX_NODES_FACTOR 5
#define MAX_STACK_DEPTH 64
#define THETA_DEFAULT 1.0f

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
__global__ void compute_forces_kernel(float *px, float *py, float *mass,
                                       float *fx, float *fy,
                                       TreeNode *tree, int num_bodies,
                                       int root, float theta, float eps2) {
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
__global__ void update_bodies_kernel(float *px, float *py,
                                      float *vx, float *vy,
                                      float *fx, float *fy,
                                      float *mass, int num_bodies,
                                      float dt) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    if (tid >= num_bodies) return;

    float ax = fx[tid] / mass[tid];
    float ay = fy[tid] / mass[tid];
    vx[tid] += ax * dt;
    vy[tid] += ay * dt;
    px[tid] += vx[tid] * dt;
    py[tid] += vy[tid] * dt;
}

// Host quadtree builder
struct QuadTreeBuilder {
    TreeNode *nodes;
    int num_nodes;
    int max_nodes;

    int allocNode() {
        if (num_nodes >= max_nodes) return -1;
        int idx = num_nodes++;
        nodes[idx].cx = 0; nodes[idx].cy = 0;
        nodes[idx].mass = 0;
        nodes[idx].body_id = -1;
        for (int c = 0; c < 4; c++) nodes[idx].children[c] = -1;
        return idx;
    }

    // Determine which quadrant a point falls in
    int quadrant(float x, float y, float mid_x, float mid_y) {
        int q = 0;
        if (x >= mid_x) q |= 1;
        if (y >= mid_y) q |= 2;
        return q;
    }

    void insert(int node_idx, int body_idx, float bx, float by, float bmass,
                float xmin, float ymin, float xmax, float ymax, int depth) {
        if (node_idx < 0 || depth > 30) return;

        TreeNode &node = nodes[node_idx];
        node.x_min = xmin; node.y_min = ymin;
        node.x_max = xmax; node.y_max = ymax;

        float mid_x = (xmin + xmax) * 0.5f;
        float mid_y = (ymin + ymax) * 0.5f;

        if (node.mass == 0.0f && node.body_id < 0) {
            // Empty node -- place body here as leaf
            node.cx = bx;
            node.cy = by;
            node.mass = bmass;
            node.body_id = body_idx;
            return;
        }

        if (node.body_id >= 0) {
            // Leaf with existing body -- need to split
            int old_body = node.body_id;
            float old_cx = node.cx;
            float old_cy = node.cy;
            float old_mass = node.mass;
            node.body_id = -1;  // now internal

            // Re-insert existing body
            int q1 = quadrant(old_cx, old_cy, mid_x, mid_y);
            if (node.children[q1] < 0) {
                node.children[q1] = allocNode();
            }
            float cxmin, cymin, cxmax, cymax;
            childBounds(q1, xmin, ymin, xmax, ymax, mid_x, mid_y,
                        cxmin, cymin, cxmax, cymax);
            insert(node.children[q1], old_body, old_cx, old_cy, old_mass,
                   cxmin, cymin, cxmax, cymax, depth + 1);
        }

        // Insert new body
        int q2 = quadrant(bx, by, mid_x, mid_y);
        if (node.children[q2] < 0) {
            node.children[q2] = allocNode();
        }
        float cxmin, cymin, cxmax, cymax;
        childBounds(q2, xmin, ymin, xmax, ymax, mid_x, mid_y,
                    cxmin, cymin, cxmax, cymax);
        insert(node.children[q2], body_idx, bx, by, bmass,
               cxmin, cymin, cxmax, cymax, depth + 1);

        // Update center of mass
        float total_mass = 0.0f;
        float wcx = 0.0f, wcy = 0.0f;
        for (int c = 0; c < 4; c++) {
            if (node.children[c] >= 0) {
                TreeNode &child = nodes[node.children[c]];
                total_mass += child.mass;
                wcx += child.mass * child.cx;
                wcy += child.mass * child.cy;
            }
        }
        if (total_mass > 0.0f) {
            node.cx = wcx / total_mass;
            node.cy = wcy / total_mass;
            node.mass = total_mass;
        }
    }

    void childBounds(int q, float xmin, float ymin, float xmax, float ymax,
                     float mid_x, float mid_y,
                     float &cxmin, float &cymin, float &cxmax, float &cymax) {
        cxmin = (q & 1) ? mid_x : xmin;
        cxmax = (q & 1) ? xmax : mid_x;
        cymin = (q & 2) ? mid_y : ymin;
        cymax = (q & 2) ? ymax : mid_y;
    }
};

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_bodies = parseIntParam(argc, argv, "--num_bodies", 16384);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int num_timesteps = parseIntParam(argc, argv, "--num_timesteps", 1);
    // theta parsed as integer * 0.1 for simplicity (e.g., 10 = 1.0)
    int theta_x10 = parseIntParam(argc, argv, "--theta_x10", 10);
    float theta = theta_x10 * 0.1f;

    float dt = 0.01f;
    float eps2 = 0.01f;  // softening parameter

    int max_tree_nodes = num_bodies * MAX_NODES_FACTOR;

    // Host body arrays
    float *h_px = (float *)malloc(num_bodies * sizeof(float));
    float *h_py = (float *)malloc(num_bodies * sizeof(float));
    float *h_vx = (float *)malloc(num_bodies * sizeof(float));
    float *h_vy = (float *)malloc(num_bodies * sizeof(float));
    float *h_mass = (float *)malloc(num_bodies * sizeof(float));

    // Generate random bodies
    srand(42);
    for (int i = 0; i < num_bodies; i++) {
        h_px[i] = (float)(rand() % 10000) / 10000.0f * 100.0f;
        h_py[i] = (float)(rand() % 10000) / 10000.0f * 100.0f;
        h_vx[i] = 0.0f;
        h_vy[i] = 0.0f;
        h_mass[i] = 1.0f + (float)(rand() % 100) / 100.0f;
    }

    // Host tree node array
    TreeNode *h_tree = (TreeNode *)malloc(max_tree_nodes * sizeof(TreeNode));

    // Device allocations
    float *d_px, *d_py, *d_vx, *d_vy, *d_mass, *d_fx, *d_fy;
    TreeNode *d_tree;
    CUDA_CHECK(cudaMalloc(&d_px, num_bodies * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_py, num_bodies * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_vx, num_bodies * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_vy, num_bodies * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_mass, num_bodies * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_fx, num_bodies * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_fy, num_bodies * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_tree, max_tree_nodes * sizeof(TreeNode)));

    dim3 block(block_size);
    dim3 grid((num_bodies + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_bodies);

    printCSVHeader();

    BenchResult r = runBenchmark("bh", problemSize, iterations, [&]() {
        // Reset body positions for each benchmark iteration
        for (int i = 0; i < num_bodies; i++) {
            h_px[i] = (float)(rand() % 10000) / 10000.0f * 100.0f;
            h_py[i] = (float)(rand() % 10000) / 10000.0f * 100.0f;
            h_vx[i] = 0.0f;
            h_vy[i] = 0.0f;
        }

        CUDA_CHECK(cudaMemcpy(d_px, h_px, num_bodies * sizeof(float),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(d_py, h_py, num_bodies * sizeof(float),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(d_vx, h_vx, num_bodies * sizeof(float),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(d_vy, h_vy, num_bodies * sizeof(float),
                              cudaMemcpyHostToDevice));
        CUDA_CHECK(cudaMemcpy(d_mass, h_mass, num_bodies * sizeof(float),
                              cudaMemcpyHostToDevice));

        for (int step = 0; step < num_timesteps; step++) {
            // Copy positions back to host for tree building
            if (step > 0) {
                CUDA_CHECK(cudaMemcpy(h_px, d_px, num_bodies * sizeof(float),
                                      cudaMemcpyDeviceToHost));
                CUDA_CHECK(cudaMemcpy(h_py, d_py, num_bodies * sizeof(float),
                                      cudaMemcpyDeviceToHost));
            }

            // Build quadtree on host
            QuadTreeBuilder builder;
            builder.nodes = h_tree;
            builder.num_nodes = 0;
            builder.max_nodes = max_tree_nodes;

            int root = builder.allocNode();
            h_tree[root].x_min = 0; h_tree[root].y_min = 0;
            h_tree[root].x_max = 100; h_tree[root].y_max = 100;

            for (int i = 0; i < num_bodies; i++) {
                // Clamp to domain
                float bx = fminf(fmaxf(h_px[i], 0.001f), 99.999f);
                float by = fminf(fmaxf(h_py[i], 0.001f), 99.999f);
                builder.insert(root, i, bx, by, h_mass[i],
                               0.0f, 0.0f, 100.0f, 100.0f, 0);
            }

            // Copy tree to device
            CUDA_CHECK(cudaMemcpy(d_tree, h_tree,
                                  builder.num_nodes * sizeof(TreeNode),
                                  cudaMemcpyHostToDevice));

            // Compute forces on GPU
            compute_forces_kernel<<<grid, block>>>(
                d_px, d_py, d_mass, d_fx, d_fy,
                d_tree, num_bodies, root, theta, eps2);
            CUDA_CHECK(cudaDeviceSynchronize());

            // Update positions on GPU
            update_bodies_kernel<<<grid, block>>>(
                d_px, d_py, d_vx, d_vy, d_fx, d_fy,
                d_mass, num_bodies, dt);
            CUDA_CHECK(cudaDeviceSynchronize());
        }
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_px));
    CUDA_CHECK(cudaFree(d_py));
    CUDA_CHECK(cudaFree(d_vx));
    CUDA_CHECK(cudaFree(d_vy));
    CUDA_CHECK(cudaFree(d_mass));
    CUDA_CHECK(cudaFree(d_fx));
    CUDA_CHECK(cudaFree(d_fy));
    CUDA_CHECK(cudaFree(d_tree));
    free(h_px);
    free(h_py);
    free(h_vx);
    free(h_vy);
    free(h_mass);
    free(h_tree);

    return 0;
}
