// attention.cpp -- HIP naive multi-head attention benchmark
// Implements: Attention(Q, K, V) = softmax(Q * K^T / sqrt(head_dim)) * V
// This is the NAIVE version -- materializes the full O(seq^2) attention matrix.
// Reports GFLOPS = (4 * batch * heads * seq^2 * head_dim) / (time_sec * 1e9)

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Kernel 1: QK^T matmul (batched)
// scores[b][h][i][j] = sum_d(Q[b][h][i][d] * K[b][h][j][d]) * scale
// Each thread computes one element of the score matrix.
// ---------------------------------------------------------------------------
__global__ void qk_matmul_kernel(const float* __restrict__ Q,
                                  const float* __restrict__ K,
                                  float* __restrict__ scores,
                                  int batch_size, int num_heads,
                                  int seq_len, int head_dim, float scale) {
    int total = batch_size * num_heads * seq_len * seq_len;
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (gid >= total) return;

    // Decode: gid -> (batch, head, row, col)
    int col = gid % seq_len;
    int tmp = gid / seq_len;
    int row = tmp % seq_len;
    tmp = tmp / seq_len;
    int head = tmp % num_heads;
    int batch = tmp / num_heads;

    // Q[batch][head][row][d], K[batch][head][col][d]
    int q_base = ((batch * num_heads + head) * seq_len + row) * head_dim;
    int k_base = ((batch * num_heads + head) * seq_len + col) * head_dim;

    float acc = 0.0f;
    for (int d = 0; d < head_dim; d++) {
        acc += Q[q_base + d] * K[k_base + d];
    }

    scores[gid] = acc * scale;
}

// ---------------------------------------------------------------------------
// Kernel 2: Row-wise softmax on scores
// Each thread handles one row of the [seq_len x seq_len] score matrix.
// ---------------------------------------------------------------------------
__global__ void softmax_kernel(float* scores, int total_rows, int row_size) {
    int row_idx = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (row_idx >= total_rows) return;

    float* row = scores + row_idx * row_size;

    // Find max for numerical stability
    float max_val = row[0];
    for (int j = 1; j < row_size; j++) {
        if (row[j] > max_val) max_val = row[j];
    }

    // Compute exp and sum
    float sum = 0.0f;
    for (int j = 0; j < row_size; j++) {
        row[j] = expf(row[j] - max_val);
        sum += row[j];
    }

    // Normalize
    float inv_sum = 1.0f / sum;
    for (int j = 0; j < row_size; j++) {
        row[j] *= inv_sum;
    }
}

// ---------------------------------------------------------------------------
// Kernel 3: Attention * V matmul
// output[b][h][i][d] = sum_j(attn_weights[b][h][i][j] * V[b][h][j][d])
// Each thread computes one element of the output.
// ---------------------------------------------------------------------------
__global__ void attn_v_matmul_kernel(const float* __restrict__ attn_weights,
                                      const float* __restrict__ V,
                                      float* __restrict__ output,
                                      int batch_size, int num_heads,
                                      int seq_len, int head_dim) {
    int total = batch_size * num_heads * seq_len * head_dim;
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (gid >= total) return;

    // Decode: gid -> (batch, head, row, dim)
    int dim = gid % head_dim;
    int tmp = gid / head_dim;
    int row = tmp % seq_len;
    tmp = tmp / seq_len;
    int head = tmp % num_heads;
    int batch = tmp / num_heads;

    // attn_weights[batch][head][row][j], V[batch][head][j][dim]
    int attn_base = ((batch * num_heads + head) * seq_len + row) * seq_len;
    int v_offset  = ((batch * num_heads + head) * seq_len) * head_dim;

    float acc = 0.0f;
    for (int j = 0; j < seq_len; j++) {
        acc += attn_weights[attn_base + j] * V[v_offset + j * head_dim + dim];
    }

    output[gid] = acc;
}

// ---------------------------------------------------------------------------
// Decode head_config to num_heads and head_dim
// ---------------------------------------------------------------------------
static void decodeHeadConfig(int config, int &num_heads, int &head_dim) {
    switch (config) {
        case 1: num_heads = 12; head_dim = 64;  break; // GPT-2
        case 2: num_heads = 32; head_dim = 128; break; // Llama-2-7B
        case 3: num_heads = 64; head_dim = 128; break; // Llama-2-70B
        default:
            fprintf(stderr, "Unknown head_config %d. Use 1-3.\n", config);
            exit(1);
    }
}

int main(int argc, char** argv) {
    int iterations  = parseIterations(argc, argv);
    int batch_size  = parseIntParam(argc, argv, "--batch_size", 4);
    int seq_len     = parseIntParam(argc, argv, "--seq_len", 256);
    int head_config = parseIntParam(argc, argv, "--head_config", 1);
    int block_size  = parseIntParam(argc, argv, "--block_size", 256);

    int num_heads, head_dim;
    decodeHeadConfig(head_config, num_heads, head_dim);

    float scale = 1.0f / sqrtf((float)head_dim);

    // Tensor sizes
    size_t qkv_elements = (size_t)batch_size * num_heads * seq_len * head_dim;
    size_t qkv_bytes    = qkv_elements * sizeof(float);
    size_t score_elements = (size_t)batch_size * num_heads * seq_len * seq_len;
    size_t score_bytes    = score_elements * sizeof(float);

    // Allocate host memory
    float* h_Q = (float*)malloc(qkv_bytes);
    float* h_K = (float*)malloc(qkv_bytes);
    float* h_V = (float*)malloc(qkv_bytes);

    // Initialize with deterministic random data
    srand(42);
    for (size_t i = 0; i < qkv_elements; i++) {
        h_Q[i] = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
        h_K[i] = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
        h_V[i] = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
    }

    // Allocate device memory
    float *d_Q, *d_K, *d_V, *d_scores, *d_output;
    HIP_CHECK(hipMalloc(&d_Q, qkv_bytes));
    HIP_CHECK(hipMalloc(&d_K, qkv_bytes));
    HIP_CHECK(hipMalloc(&d_V, qkv_bytes));
    HIP_CHECK(hipMalloc(&d_scores, score_bytes));
    HIP_CHECK(hipMalloc(&d_output, qkv_bytes));

    HIP_CHECK(hipMemcpy(d_Q, h_Q, qkv_bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_K, h_K, qkv_bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_V, h_V, qkv_bytes, hipMemcpyHostToDevice));

    // Launch configurations
    int qk_total  = batch_size * num_heads * seq_len * seq_len;
    int qk_grid   = (qk_total + block_size - 1) / block_size;

    int softmax_total = batch_size * num_heads * seq_len; // one row per thread
    int softmax_grid  = (softmax_total + block_size - 1) / block_size;

    int av_total  = batch_size * num_heads * seq_len * head_dim;
    int av_grid   = (av_total + block_size - 1) / block_size;

    // Problem size string
    char problemSize[256];
    snprintf(problemSize, sizeof(problemSize),
             "B%d_S%d_H%d_D%d_blk%d",
             batch_size, seq_len, num_heads, head_dim, block_size);

    printCSVHeader();

    BenchResult r = runBenchmark("naive_attention", problemSize, iterations, [&]() {
        hipLaunchKernelGGL(qk_matmul_kernel, dim3(qk_grid), dim3(block_size),
            0, 0, d_Q, d_K, d_scores,
            batch_size, num_heads, seq_len, head_dim, scale);
        hipLaunchKernelGGL(softmax_kernel, dim3(softmax_grid), dim3(block_size),
            0, 0, d_scores, softmax_total, seq_len);
        hipLaunchKernelGGL(attn_v_matmul_kernel, dim3(av_grid), dim3(block_size),
            0, 0, d_scores, d_V, d_output,
            batch_size, num_heads, seq_len, head_dim);
    });

    printCSVRow(r);

    // GFLOPS: QK^T = 2*B*H*S*S*D, attn*V = 2*B*H*S*S*D, total = 4*B*H*S*S*D
    double total_flops = 4.0 * (double)batch_size * num_heads *
                         (double)seq_len * seq_len * head_dim;
    double gflops = (total_flops / 1.0e9) / (r.avg_ms / 1000.0);
    printf("# batch=%d, seq_len=%d, num_heads=%d, head_dim=%d, block_size=%d\n",
           batch_size, seq_len, num_heads, head_dim, block_size);
    printf("# Total FLOPs: %.2e\n", total_flops);
    printf("# Throughput: %.2f GFLOPS\n", gflops);

    // Cleanup
    HIP_CHECK(hipFree(d_Q));
    HIP_CHECK(hipFree(d_K));
    HIP_CHECK(hipFree(d_V));
    HIP_CHECK(hipFree(d_scores));
    HIP_CHECK(hipFree(d_output));
    free(h_Q);
    free(h_K);
    free(h_V);

    return 0;
}
