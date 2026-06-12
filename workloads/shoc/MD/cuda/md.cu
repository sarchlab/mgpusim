// md.cu -- SHOC MD benchmark (CUDA)
// Lennard-Jones molecular dynamics force computation.
// O(N^2) all-pairs, no neighbor lists.

#include "bench_common_cuda.h"

// ---------------------------------------------------------------------------
// Lennard-Jones constants
// ---------------------------------------------------------------------------
#define LJ_EPSILON  1.0f
#define LJ_SIGMA    1.0f
#define BOX_SIZE    100.0f

// ---------------------------------------------------------------------------
// Kernel: compute LJ force on each atom from all other atoms within cutoff
// ---------------------------------------------------------------------------
__global__ void md_kernel(const float4* __restrict__ pos,
                          float4* __restrict__ force,
                          int N, float cutoff) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i >= N) return;

    float4 pi = pos[i];
    float fx = 0.0f, fy = 0.0f, fz = 0.0f;
    float cutoff2 = cutoff * cutoff;
    float sigma6 = LJ_SIGMA * LJ_SIGMA * LJ_SIGMA *
                   LJ_SIGMA * LJ_SIGMA * LJ_SIGMA;
    float sigma12 = sigma6 * sigma6;

    for (int j = 0; j < N; ++j) {
        if (i == j) continue;

        float4 pj = pos[j];
        float dx = pj.x - pi.x;
        float dy = pj.y - pi.y;
        float dz = pj.z - pi.z;

        float r2 = dx * dx + dy * dy + dz * dz;

        if (r2 < cutoff2 && r2 > 1e-10f) {
            float r2inv = 1.0f / r2;
            float r6inv = r2inv * r2inv * r2inv;
            float r = sqrtf(r2);
            float rinv = 1.0f / r;

            // F_ij = 48*epsilon * (sigma^12/r^13 - 0.5*sigma^6/r^7) * r_vec
            float f = 48.0f * LJ_EPSILON * (sigma12 * r6inv * r6inv * rinv -
                      0.5f * sigma6 * r6inv * rinv) * rinv;

            fx += f * dx;
            fy += f * dy;
            fz += f * dz;
        }
    }

    force[i] = make_float4(fx, fy, fz, 0.0f);
}

// ---------------------------------------------------------------------------
// CPU reference for verification
// ---------------------------------------------------------------------------
static void md_cpu(const float4* pos, float4* force, int N, float cutoff) {
    float cutoff2 = cutoff * cutoff;
    float sigma6 = LJ_SIGMA * LJ_SIGMA * LJ_SIGMA *
                   LJ_SIGMA * LJ_SIGMA * LJ_SIGMA;
    float sigma12 = sigma6 * sigma6;

    for (int i = 0; i < N; ++i) {
        float fx = 0.0f, fy = 0.0f, fz = 0.0f;
        for (int j = 0; j < N; ++j) {
            if (i == j) continue;
            float dx = pos[j].x - pos[i].x;
            float dy = pos[j].y - pos[i].y;
            float dz = pos[j].z - pos[i].z;
            float r2 = dx * dx + dy * dy + dz * dz;
            if (r2 < cutoff2 && r2 > 1e-10f) {
                float r2inv = 1.0f / r2;
                float r6inv = r2inv * r2inv * r2inv;
                float r = sqrtf(r2);
                float rinv = 1.0f / r;
                float f = 48.0f * LJ_EPSILON * (sigma12 * r6inv * r6inv * rinv -
                          0.5f * sigma6 * r6inv * rinv) * rinv;
                fx += f * dx;
                fy += f * dy;
                fz += f * dz;
            }
        }
        force[i] = make_float4(fx, fy, fz, 0.0f);
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--size", 4096);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int iters      = parseIterations(argc, argv);
    // Parse cutoff as int (x10), default 100 -> 10.0f
    int cutoff_int = parseIntParam(argc, argv, "--cutoff_x10", 100);
    float cutoff   = (float)cutoff_int * 0.1f;

    size_t bytesPos   = (size_t)N * sizeof(float4);
    size_t bytesForce = (size_t)N * sizeof(float4);

    // Host allocations
    float4* h_pos   = (float4*)malloc(bytesPos);
    float4* h_force = (float4*)malloc(bytesForce);
    float4* h_ref   = (float4*)malloc(bytesForce);

    // Initialize random positions in box
    srand(42);
    for (int i = 0; i < N; ++i) {
        h_pos[i].x = ((float)rand() / RAND_MAX) * BOX_SIZE;
        h_pos[i].y = ((float)rand() / RAND_MAX) * BOX_SIZE;
        h_pos[i].z = ((float)rand() / RAND_MAX) * BOX_SIZE;
        h_pos[i].w = 0.0f;
    }

    // Device allocations
    float4 *d_pos, *d_force;
    CUDA_CHECK(cudaMalloc(&d_pos, bytesPos));
    CUDA_CHECK(cudaMalloc(&d_force, bytesForce));

    CUDA_CHECK(cudaMemcpy(d_pos, h_pos, bytesPos, cudaMemcpyHostToDevice));

    int grid = (N + block_size - 1) / block_size;

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("md", size_str, iters, [&]() {
        md_kernel<<<grid, block_size>>>(d_pos, d_force, N, cutoff);
    });

    printCSVRow(r);

    // Verify (only for small sizes)
    if (N <= 1024) {
        CUDA_CHECK(cudaMemcpy(h_force, d_force, bytesForce,
                              cudaMemcpyDeviceToHost));
        md_cpu(h_pos, h_ref, N, cutoff);

        int errors = 0;
        for (int i = 0; i < N; ++i) {
            float dx = fabsf(h_force[i].x - h_ref[i].x);
            float dy = fabsf(h_force[i].y - h_ref[i].y);
            float dz = fabsf(h_force[i].z - h_ref[i].z);
            float mag = sqrtf(h_ref[i].x * h_ref[i].x +
                              h_ref[i].y * h_ref[i].y +
                              h_ref[i].z * h_ref[i].z);
            float tol = 1e-2f * mag + 1e-4f;
            if (dx > tol || dy > tol || dz > tol) {
                if (errors < 10) {
                    fprintf(stderr,
                            "Mismatch at %d: got (%.4f,%.4f,%.4f), "
                            "expected (%.4f,%.4f,%.4f)\n",
                            i, h_force[i].x, h_force[i].y, h_force[i].z,
                            h_ref[i].x, h_ref[i].y, h_ref[i].z);
                }
                errors++;
            }
        }
        if (errors > 0) {
            fprintf(stderr, "FAIL: %d errors\n", errors);
        } else {
            fprintf(stderr, "PASS\n");
        }
    } else {
        fprintf(stderr, "Skipping verification for large size\n");
    }

    // Cleanup
    CUDA_CHECK(cudaFree(d_pos));
    CUDA_CHECK(cudaFree(d_force));
    free(h_pos);
    free(h_force);
    free(h_ref);

    return 0;
}
