// rope.cpp -- HIP Rotary Position Embedding (RoPE) benchmark
// Applies rotation matrices to Q and K vectors using sinusoidal
// position-dependent frequencies. Used in Llama-2, GPT-NeoX, etc.
// Reports bandwidth in GB/s: (4 * batch * seq * heads * head_dim * sizeof(float)) / time

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// RoPE kernel -- applies rotary embeddings to Q and K tensors
// ---------------------------------------------------------------------------
__global__ void rope_kernel(float* q, float* k,
    int batch_size, int seq_len, int num_heads, int head_dim,
    float theta_base) {
    int half_head = head_dim / 2;
    int total = batch_size * seq_len * num_heads * half_head;
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (gid >= total) return;

    // Decode indices
    int dim_pair = gid % half_head;
    int tmp = gid / half_head;
    int head = tmp % num_heads;
    tmp = tmp / num_heads;
    int pos = tmp % seq_len;
    int batch = tmp / seq_len;

    // Compute rotation angle
    float freq = 1.0f / powf(theta_base, (2.0f * dim_pair) / head_dim);
    float angle = pos * freq;
    float cos_val = cosf(angle);
    float sin_val = sinf(angle);

    // Apply rotation to Q
    int base_idx = ((batch * seq_len + pos) * num_heads + head) * head_dim;
    int idx0 = base_idx + 2 * dim_pair;
    int idx1 = base_idx + 2 * dim_pair + 1;

    float q0 = q[idx0], q1 = q[idx1];
    q[idx0] = q0 * cos_val - q1 * sin_val;
    q[idx1] = q0 * sin_val + q1 * cos_val;

    // Apply same rotation to K
    float k0 = k[idx0], k1 = k[idx1];
    k[idx0] = k0 * cos_val - k1 * sin_val;
    k[idx1] = k0 * sin_val + k1 * cos_val;
}

// ---------------------------------------------------------------------------
// Decode head_config to num_heads and head_dim
// ---------------------------------------------------------------------------
static void decodeHeadConfig(int config, int &num_heads, int &head_dim) {
    switch (config) {
        case 1: num_heads = 12; head_dim = 64;  break; // GPT-2
        case 2: num_heads = 32; head_dim = 128; break; // Llama-2-7B
        case 3: num_heads = 40; head_dim = 128; break; // Llama-2-13B
        case 4: num_heads = 64; head_dim = 128; break; // Llama-2-70B
        default:
            fprintf(stderr, "Unknown head_config %d. Use 1-4.\n", config);
            exit(1);
    }
}

int main(int argc, char** argv) {
    int iterations  = parseIterations(argc, argv);
    int batch_size  = parseIntParam(argc, argv, "--batch_size", 4);
    int seq_len     = parseIntParam(argc, argv, "--seq_len", 1024);
    int head_config = parseIntParam(argc, argv, "--head_config", 2);
    int block_size  = parseIntParam(argc, argv, "--block_size", 256);

    int num_heads, head_dim;
    decodeHeadConfig(head_config, num_heads, head_dim);

    float theta_base = 10000.0f;

    // Total elements in Q (and K) tensor
    size_t num_elements = (size_t)batch_size * seq_len * num_heads * head_dim;
    size_t tensor_bytes = num_elements * sizeof(float);

    // Allocate host memory
    float* h_q = (float*)malloc(tensor_bytes);
    float* h_k = (float*)malloc(tensor_bytes);

    // Initialize with deterministic random data
    srand(42);
    for (size_t i = 0; i < num_elements; i++) {
        h_q[i] = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
        h_k[i] = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
    }

    // Allocate device memory
    float *d_q, *d_k;
    HIP_CHECK(hipMalloc(&d_q, tensor_bytes));
    HIP_CHECK(hipMalloc(&d_k, tensor_bytes));
    HIP_CHECK(hipMemcpy(d_q, h_q, tensor_bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_k, h_k, tensor_bytes, hipMemcpyHostToDevice));

    // Launch configuration
    int half_head = head_dim / 2;
    int total_threads = batch_size * seq_len * num_heads * half_head;
    int grid_size = (total_threads + block_size - 1) / block_size;

    // Problem size string
    char problemSize[256];
    snprintf(problemSize, sizeof(problemSize),
             "B%d_S%d_H%d_D%d_blk%d",
             batch_size, seq_len, num_heads, head_dim, block_size);

    printCSVHeader();

    BenchResult r = runBenchmark("rope", problemSize, iterations, [&]() {
        hipLaunchKernelGGL(rope_kernel, dim3(grid_size), dim3(block_size),
            0, 0, d_q, d_k,
            batch_size, seq_len, num_heads, head_dim, theta_base);
    });

    printCSVRow(r);

    // Bandwidth: read Q+K, write Q+K = 4 tensors worth of data
    double total_bytes = 4.0 * (double)num_elements * sizeof(float);
    double gb_per_sec = (total_bytes / 1.0e9) / (r.avg_ms / 1000.0);
    printf("# batch=%d, seq_len=%d, num_heads=%d, head_dim=%d, block_size=%d\n",
           batch_size, seq_len, num_heads, head_dim, block_size);
    printf("# Total bytes transferred: %.2e\n", total_bytes);
    printf("# Bandwidth: %.2f GB/s\n", gb_per_sec);

    // Cleanup
    HIP_CHECK(hipFree(d_q));
    HIP_CHECK(hipFree(d_k));
    free(h_q);
    free(h_k);

    return 0;
}
