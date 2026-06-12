#include <hip/hip_runtime.h>

// Rotary Position Embedding (RoPE):
// For each token position pos, dimension pair (2i, 2i+1):
//   theta = pos / 10000^(2i / head_dim)
//   out[2i]   = x[2i]*cos(theta) - x[2i+1]*sin(theta)
//   out[2i+1] = x[2i]*sin(theta) + x[2i+1]*cos(theta)
//
// Input layout: [batch, seq_len, num_heads, head_dim]
// Each thread processes one pair of dimensions (2i, 2i+1).

__global__ void rope_kernel(const float* __restrict__ X,
                            float* __restrict__ Y,
                            int batch,
                            int seq_len,
                            int num_heads,
                            int head_dim) {
    // Total pairs = batch * seq_len * num_heads * (head_dim / 2)
    int half_dim = head_dim / 2;
    int total_pairs = batch * seq_len * num_heads * half_dim;

    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid >= total_pairs) return;

    // Decompose linear index to (b, s, h, i) where i is the pair index
    int tmp = gid;
    int i = tmp % half_dim;
    tmp /= half_dim;
    int h = tmp % num_heads;
    tmp /= num_heads;
    int s = tmp % seq_len;
    int b = tmp / seq_len;

    // Position in the flattened array
    int base = ((b * seq_len + s) * num_heads + h) * head_dim + 2 * i;

    float x0 = X[base];
    float x1 = X[base + 1];

    // Compute frequency: theta = pos / 10000^(2i / head_dim)
    float freq = 1.0f / powf(10000.0f, (2.0f * (float)i) / (float)head_dim);
    float theta = (float)s * freq;

    float cos_t = cosf(theta);
    float sin_t = sinf(theta);

    Y[base]     = x0 * cos_t - x1 * sin_t;
    Y[base + 1] = x0 * sin_t + x1 * cos_t;
}
