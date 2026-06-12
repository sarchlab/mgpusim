#include <hip/hip_runtime.h>

// Naive attention: Q*K^T / sqrt(d) -> softmax -> * V
// Q, K, V: [batch*num_heads, seq_len, head_dim]
// Output:  [batch*num_heads, seq_len, head_dim]
//
// Each workgroup processes one (batch_head, query_row) pair.
// blockDim.x threads cooperate via shared memory reduction.
#define MAX_BLOCK_SIZE 256

__global__ void naive_attention_kernel(
    const float* __restrict__ Q,
    const float* __restrict__ K,
    const float* __restrict__ V,
    float* __restrict__ Out,
    int batch_heads,
    int seq_len,
    int head_dim) {

    __shared__ float sdata[MAX_BLOCK_SIZE];

    int bh = blockIdx.x;  // batch_head index
    int qr = blockIdx.y;  // query row index

    if (bh >= batch_heads || qr >= seq_len) return;

    int tid = threadIdx.x;
    int blockSize = blockDim.x;

    float scale = rsqrtf((float)head_dim);

    // Pointers for this batch_head
    const float* Q_row = Q + (bh * seq_len + qr) * head_dim;
    const float* K_bh = K + bh * seq_len * head_dim;
    const float* V_bh = V + bh * seq_len * head_dim;
    float* Out_row = Out + (bh * seq_len + qr) * head_dim;

    // Phase 1: Compute QK^T scores and find max for numerical stability
    // We process seq_len key columns. Each thread handles a subset.
    // We need to store all scores, so we compute in passes.

    // First pass: compute scores and find max
    float localMax = -1e30f;
    for (int kc = tid; kc < seq_len; kc += blockSize) {
        float dot = 0.0f;
        for (int d = 0; d < head_dim; d++) {
            dot += Q_row[d] * K_bh[kc * head_dim + d];
        }
        dot *= scale;
        if (dot > localMax) localMax = dot;
    }
    sdata[tid] = localMax;
    __syncthreads();

    for (int s = blockSize / 2; s > 0; s >>= 1) {
        if (tid < s) {
            if (sdata[tid + s] > sdata[tid])
                sdata[tid] = sdata[tid + s];
        }
        __syncthreads();
    }
    float rowMax = sdata[0];
    __syncthreads();

    // Second pass: compute exp(score - max) and sum
    float localSum = 0.0f;
    for (int kc = tid; kc < seq_len; kc += blockSize) {
        float dot = 0.0f;
        for (int d = 0; d < head_dim; d++) {
            dot += Q_row[d] * K_bh[kc * head_dim + d];
        }
        dot *= scale;
        localSum += expf(dot - rowMax);
    }
    sdata[tid] = localSum;
    __syncthreads();

    for (int s = blockSize / 2; s > 0; s >>= 1) {
        if (tid < s) {
            sdata[tid] += sdata[tid + s];
        }
        __syncthreads();
    }
    float rowSum = sdata[0];
    float invSum = 1.0f / rowSum;
    __syncthreads();

    // Phase 2: Compute weighted sum of V rows
    // For each output dimension d, accumulate softmax(score) * V[kc][d]
    for (int d = tid; d < head_dim; d += blockSize) {
        float acc = 0.0f;
        for (int kc = 0; kc < seq_len; kc++) {
            // Recompute score for this key column
            float dot = 0.0f;
            for (int dd = 0; dd < head_dim; dd++) {
                dot += Q_row[dd] * K_bh[kc * head_dim + dd];
            }
            dot *= scale;
            float w = expf(dot - rowMax) * invSum;
            acc += w * V_bh[kc * head_dim + d];
        }
        Out_row[d] = acc;
    }
}
