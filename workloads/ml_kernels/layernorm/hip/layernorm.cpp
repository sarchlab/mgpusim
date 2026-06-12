// layernorm.cpp -- HIP LayerNorm / RMSNorm benchmark
// Implements both LayerNorm and RMSNorm (selectable via --norm_type).
// - LayerNorm: y = gamma * (x - mean) / sqrt(var + eps) + beta
// - RMSNorm:   y = gamma * x / sqrt(mean(x^2) + eps)
// One block per row. Uses warp shuffle + shared memory reductions.
// Reports bandwidth in GB/s.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Warp-level reduction helper
// ---------------------------------------------------------------------------
__inline__ __device__ float warpReduceSum(float val) {
    for (int offset = warpSize / 2; offset > 0; offset >>= 1) {
        val += __shfl_down(val, offset);
    }
    return val;
}

// ---------------------------------------------------------------------------
// Block-level reduction using shared memory + warp shuffle
// ---------------------------------------------------------------------------
__inline__ __device__ float blockReduceSum(float val) {
    __shared__ float shared[32]; // one slot per warp
    int lane = hipThreadIdx_x % warpSize;
    int wid  = hipThreadIdx_x / warpSize;

    val = warpReduceSum(val);

    if (lane == 0) shared[wid] = val;
    __syncthreads();

    int numWarps = (hipBlockDim_x + warpSize - 1) / warpSize;
    val = (hipThreadIdx_x < numWarps) ? shared[hipThreadIdx_x] : 0.0f;
    if (wid == 0) val = warpReduceSum(val);

    return val;
}

// ---------------------------------------------------------------------------
// LayerNorm / RMSNorm kernel -- one block per row
// norm_type: 1 = LayerNorm, 2 = RMSNorm, 3 = RMSNorm without learnable scale
// ---------------------------------------------------------------------------
__global__ void layernorm_kernel(const float* __restrict__ input,
                                 const float* __restrict__ gamma,
                                 const float* __restrict__ beta,
                                 float* __restrict__ output,
                                 int num_rows, int hidden_dim,
                                 float eps, int norm_type) {
    int row = hipBlockIdx_x;
    if (row >= num_rows) return;

    const float* x = input + row * hidden_dim;
    float* y = output + row * hidden_dim;

    // Step 1: Compute sum and sum of squares via parallel reduction
    float sum = 0.0f, sum_sq = 0.0f;
    for (int i = hipThreadIdx_x; i < hidden_dim; i += hipBlockDim_x) {
        float val = x[i];
        sum += val;
        sum_sq += val * val;
    }

    sum    = blockReduceSum(sum);
    sum_sq = blockReduceSum(sum_sq);

    // Broadcast reduced values via shared memory
    __shared__ float s_mean, s_inv_std;
    if (hipThreadIdx_x == 0) {
        if (norm_type == 1) {
            // LayerNorm
            float mean = sum / hidden_dim;
            float var  = sum_sq / hidden_dim - mean * mean;
            s_mean     = mean;
            s_inv_std  = rsqrtf(var + eps);
        } else {
            // RMSNorm -- no mean subtraction
            s_mean    = 0.0f;
            s_inv_std = rsqrtf(sum_sq / hidden_dim + eps);
        }
    }
    __syncthreads();

    float mean    = s_mean;
    float inv_std = s_inv_std;

    // Step 2: Normalize and apply scale (gamma) and shift (beta)
    for (int i = hipThreadIdx_x; i < hidden_dim; i += hipBlockDim_x) {
        float val = x[i];
        if (norm_type == 1) {
            y[i] = gamma[i] * (val - mean) * inv_std + beta[i];
        } else if (norm_type == 2) {
            y[i] = gamma[i] * val * inv_std;
        } else {
            // norm_type == 3: RMSNorm without learnable scale -- just divide by RMS
            y[i] = val * inv_std;
        }
    }
}

int main(int argc, char** argv) {
    int iterations  = parseIterations(argc, argv);
    int num_rows    = parseIntParam(argc, argv, "--num_rows", 4096);
    int hidden_dim  = parseIntParam(argc, argv, "--hidden_dim", 4096);
    int block_size  = parseIntParam(argc, argv, "--block_size", 256);
    int norm_type   = parseIntParam(argc, argv, "--norm_type", 1);

    float eps = 1e-5f;

    size_t num_elements = (size_t)num_rows * hidden_dim;
    size_t data_bytes   = num_elements * sizeof(float);
    size_t param_bytes  = (size_t)hidden_dim * sizeof(float);

    // Allocate and initialize host data
    float* h_input = (float*)malloc(data_bytes);
    float* h_gamma = (float*)malloc(param_bytes);
    float* h_beta  = (float*)malloc(param_bytes);

    srand(42);
    for (size_t i = 0; i < num_elements; i++) {
        h_input[i] = ((float)rand() / (float)RAND_MAX) * 2.0f - 1.0f;
    }
    for (int i = 0; i < hidden_dim; i++) {
        h_gamma[i] = 1.0f + ((float)rand() / (float)RAND_MAX) * 0.1f;
        h_beta[i]  = ((float)rand() / (float)RAND_MAX) * 0.01f;
    }

    // Allocate device memory
    float *d_input, *d_gamma, *d_beta, *d_output;
    HIP_CHECK(hipMalloc(&d_input, data_bytes));
    HIP_CHECK(hipMalloc(&d_gamma, param_bytes));
    HIP_CHECK(hipMalloc(&d_beta, param_bytes));
    HIP_CHECK(hipMalloc(&d_output, data_bytes));

    HIP_CHECK(hipMemcpy(d_input, h_input, data_bytes,
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_gamma, h_gamma, param_bytes,
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_beta, h_beta, param_bytes,
                        hipMemcpyHostToDevice));

    // Problem size string
    const char* norm_name = (norm_type == 1) ? "layernorm" :
                            (norm_type == 2) ? "rmsnorm" : "rmsnorm_noscale";
    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "rows%d_hid%d_bs%d_%s",
             num_rows, hidden_dim, block_size, norm_name);

    printCSVHeader();

    dim3 grid(num_rows);
    dim3 block(block_size);

    BenchResult r = runBenchmark(norm_name, problemSize, iterations, [&]() {
        hipLaunchKernelGGL(layernorm_kernel, grid, block, 0, 0,
                           d_input, d_gamma, d_beta, d_output,
                           num_rows, hidden_dim, eps, norm_type);
    });

    printCSVRow(r);

    // Bandwidth: read input + write output + gamma (if norm_type 1 or 2) + beta (if LayerNorm)
    double bytes_processed = 2.0 * (double)data_bytes; // input read + output write
    if (norm_type == 1 || norm_type == 2) bytes_processed += (double)param_bytes; // gamma
    if (norm_type == 1) bytes_processed += (double)param_bytes; // beta
    double gb_per_sec = (bytes_processed / 1.0e9) / (r.avg_ms / 1000.0);

    printf("# num_rows=%d, hidden_dim=%d, block_size=%d, norm_type=%d (%s)\n",
           num_rows, hidden_dim, block_size, norm_type, norm_name);
    printf("# Total bytes processed: %.2e\n", bytes_processed);
    printf("# Bandwidth: %.2f GB/s\n", gb_per_sec);

    // Cleanup
    HIP_CHECK(hipFree(d_input));
    HIP_CHECK(hipFree(d_gamma));
    HIP_CHECK(hipFree(d_beta));
    HIP_CHECK(hipFree(d_output));
    free(h_input);
    free(h_gamma);
    free(h_beta);

    return 0;
}
