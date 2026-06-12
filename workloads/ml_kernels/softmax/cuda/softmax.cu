// softmax.cu -- CUDA online numerically-stable softmax benchmark
// Row-wise softmax: softmax(x_i) = exp(x_i - max(x)) / sum(exp(x_j - max(x)))
// Uses warp shuffle + shared memory reductions for max and sum phases.
// One block per row. Reports bandwidth in GB/s.

#include "bench_common_cuda.h"

// ---------------------------------------------------------------------------
// Warp-level reduction helpers
// ---------------------------------------------------------------------------
__inline__ __device__ float warpReduceMax(float val) {
    for (int offset = warpSize / 2; offset > 0; offset >>= 1) {
        val = fmaxf(val, __shfl_down_sync(0xFFFFFFFF, val, offset));
    }
    return val;
}

__inline__ __device__ float warpReduceSum(float val) {
    for (int offset = warpSize / 2; offset > 0; offset >>= 1) {
        val += __shfl_down_sync(0xFFFFFFFF, val, offset);
    }
    return val;
}

// ---------------------------------------------------------------------------
// Block-level reduction using shared memory + warp shuffle
// ---------------------------------------------------------------------------
__inline__ __device__ float blockReduceMax(float val) {
    __shared__ float shared[32]; // one slot per warp
    int lane = threadIdx.x % warpSize;
    int wid  = threadIdx.x / warpSize;

    val = warpReduceMax(val);

    if (lane == 0) shared[wid] = val;
    __syncthreads();

    int numWarps = (blockDim.x + warpSize - 1) / warpSize;
    val = (threadIdx.x < numWarps) ? shared[threadIdx.x] : -FLT_MAX;
    if (wid == 0) val = warpReduceMax(val);

    return val;
}

__inline__ __device__ float blockReduceSum(float val) {
    __shared__ float shared[32];
    int lane = threadIdx.x % warpSize;
    int wid  = threadIdx.x / warpSize;

    val = warpReduceSum(val);

    if (lane == 0) shared[wid] = val;
    __syncthreads();

    int numWarps = (blockDim.x + warpSize - 1) / warpSize;
    val = (threadIdx.x < numWarps) ? shared[threadIdx.x] : 0.0f;
    if (wid == 0) val = warpReduceSum(val);

    return val;
}

// ---------------------------------------------------------------------------
// Softmax kernel -- one block per row
// ---------------------------------------------------------------------------
__global__ void softmax_kernel(float* __restrict__ input,
                               float* __restrict__ output,
                               int row_size) {
    int row = blockIdx.x;
    float* row_in  = input  + row * row_size;
    float* row_out = output + row * row_size;

    // Phase 1: Find max across the row
    float local_max = -FLT_MAX;
    for (int i = threadIdx.x; i < row_size; i += blockDim.x) {
        local_max = fmaxf(local_max, row_in[i]);
    }
    float row_max = blockReduceMax(local_max);

    // Broadcast max to all threads via shared memory
    __shared__ float s_max;
    if (threadIdx.x == 0) s_max = row_max;
    __syncthreads();
    row_max = s_max;

    // Phase 2: Compute exp(x_i - max) and sum
    float local_sum = 0.0f;
    for (int i = threadIdx.x; i < row_size; i += blockDim.x) {
        float val = expf(row_in[i] - row_max);
        row_out[i] = val; // store intermediate exp values
        local_sum += val;
    }
    float row_sum = blockReduceSum(local_sum);

    // Broadcast sum
    __shared__ float s_sum;
    if (threadIdx.x == 0) s_sum = row_sum;
    __syncthreads();
    row_sum = s_sum;

    // Phase 3: Normalize
    float inv_sum = 1.0f / row_sum;
    for (int i = threadIdx.x; i < row_size; i += blockDim.x) {
        row_out[i] *= inv_sum;
    }
}

int main(int argc, char** argv) {
    int iterations  = parseIterations(argc, argv);
    int num_rows    = parseIntParam(argc, argv, "--num_rows", 4096);
    int row_size    = parseIntParam(argc, argv, "--row_size", 1024);
    int block_size  = parseIntParam(argc, argv, "--block_size", 256);
    int input_scale = parseIntParam(argc, argv, "--input_scale", 1);

    size_t num_elements = (size_t)num_rows * row_size;
    size_t data_bytes   = num_elements * sizeof(float);

    // Allocate and initialize host data
    float* h_input = (float*)malloc(data_bytes);

    srand(42);
    float scale = (float)input_scale;
    for (size_t i = 0; i < num_elements; i++) {
        h_input[i] = (((float)rand() / RAND_MAX) * 2.0f - 1.0f) * scale;
    }

    // Allocate device memory
    float *d_input, *d_output;
    CUDA_CHECK(cudaMalloc(&d_input, data_bytes));
    CUDA_CHECK(cudaMalloc(&d_output, data_bytes));

    CUDA_CHECK(cudaMemcpy(d_input, h_input, data_bytes,
                          cudaMemcpyHostToDevice));

    // Problem size string
    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "rows%d_cols%d_bs%d",
             num_rows, row_size, block_size);

    printCSVHeader();

    dim3 grid(num_rows);
    dim3 block(block_size);

    BenchResult r = runBenchmark("softmax", problemSize, iterations, [&]() {
        softmax_kernel<<<grid, block>>>(d_input, d_output, row_size);
    });

    printCSVRow(r);

    // Bandwidth: read input + write output = 2 * data_bytes
    double bytes_processed = 2.0 * (double)data_bytes;
    double gb_per_sec = (bytes_processed / 1.0e9) / (r.avg_ms / 1000.0);

    printf("# num_rows=%d, row_size=%d, block_size=%d, input_scale=%d\n",
           num_rows, row_size, block_size, input_scale);
    printf("# Total bytes processed: %.2e\n", bytes_processed);
    printf("# Bandwidth: %.2f GB/s\n", gb_per_sec);

    // Cleanup
    CUDA_CHECK(cudaFree(d_input));
    CUDA_CHECK(cudaFree(d_output));
    free(h_input);

    return 0;
}
