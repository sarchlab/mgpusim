// ep.cu -- Heteromark Exclusive Prefix Scan benchmark (CUDA)
// Block-level Blelloch scan with block-sum propagation.

#include "bench_common_cuda.h"

#define MAX_BLOCK_SIZE 512

// ---------------------------------------------------------------------------
// Kernel: Per-block exclusive Blelloch scan
// ---------------------------------------------------------------------------
__global__ void scan_block(const float* __restrict__ input,
                           float* __restrict__ output,
                           float* __restrict__ blockSums,
                           int N) {
    extern __shared__ float temp[];

    int tid = threadIdx.x;
    int blockOffset = blockIdx.x * blockDim.x * 2;
    int ai = tid;
    int bi = tid + blockDim.x;

    // Load input into shared memory
    temp[ai] = (blockOffset + ai < N) ? input[blockOffset + ai] : 0.0f;
    temp[bi] = (blockOffset + bi < N) ? input[blockOffset + bi] : 0.0f;

    int n = blockDim.x * 2;

    // Up-sweep (reduce) phase
    int offset = 1;
    for (int d = n >> 1; d > 0; d >>= 1) {
        __syncthreads();
        if (tid < d) {
            int ai2 = offset * (2 * tid + 1) - 1;
            int bi2 = offset * (2 * tid + 2) - 1;
            temp[bi2] += temp[ai2];
        }
        offset <<= 1;
    }

    // Save block sum and clear last element
    __syncthreads();
    if (tid == 0) {
        if (blockSums != nullptr) {
            blockSums[blockIdx.x] = temp[n - 1];
        }
        temp[n - 1] = 0.0f;
    }

    // Down-sweep phase
    for (int d = 1; d < n; d <<= 1) {
        offset >>= 1;
        __syncthreads();
        if (tid < d) {
            int ai2 = offset * (2 * tid + 1) - 1;
            int bi2 = offset * (2 * tid + 2) - 1;
            float t = temp[ai2];
            temp[ai2] = temp[bi2];
            temp[bi2] += t;
        }
    }

    __syncthreads();

    // Write results
    if (blockOffset + ai < N) output[blockOffset + ai] = temp[ai];
    if (blockOffset + bi < N) output[blockOffset + bi] = temp[bi];
}

// ---------------------------------------------------------------------------
// Kernel: Add block sums back to produce final exclusive scan
// ---------------------------------------------------------------------------
__global__ void add_block_sums(float* __restrict__ output,
                               const float* __restrict__ blockSums,
                               int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < N) {
        output[i] += blockSums[blockIdx.x / (blockDim.x * 2 / blockDim.x)];
    }
}

// ---------------------------------------------------------------------------
// A simpler add kernel: each element adds the prefix sum of its block sum
// ---------------------------------------------------------------------------
__global__ void add_block_prefix(float* __restrict__ data,
                                 const float* __restrict__ blockPrefix,
                                 int N,
                                 int elemsPerBlock) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < N) {
        int origBlock = i / elemsPerBlock;
        if (origBlock > 0) {
            data[i] += blockPrefix[origBlock];
        }
    }
}

// ---------------------------------------------------------------------------
// CPU reference: exclusive prefix sum
// ---------------------------------------------------------------------------
static void exclusive_scan_cpu(const float* input, float* output, int N) {
    output[0] = 0.0f;
    for (int i = 1; i < N; ++i) {
        output[i] = output[i - 1] + input[i - 1];
    }
}

// ---------------------------------------------------------------------------
// Recursive scan on GPU
// ---------------------------------------------------------------------------
static void gpu_scan(const float* d_input, float* d_output, int N,
                     int block_size) {
    int elemsPerBlock = block_size * 2;
    int numBlocks = (N + elemsPerBlock - 1) / elemsPerBlock;
    int sharedBytes = elemsPerBlock * sizeof(float);

    float* d_blockSums = nullptr;
    if (numBlocks > 1) {
        CUDA_CHECK(cudaMalloc(&d_blockSums, numBlocks * sizeof(float)));
    }

    // Pass 1: per-block scan
    scan_block<<<numBlocks, block_size, sharedBytes>>>(
        d_input, d_output, d_blockSums, N);

    if (numBlocks > 1) {
        // Recursively scan block sums
        float* d_blockPrefix;
        CUDA_CHECK(cudaMalloc(&d_blockPrefix, numBlocks * sizeof(float)));
        gpu_scan(d_blockSums, d_blockPrefix, numBlocks, block_size);

        // Add block prefixes back
        int addGrid = (N + block_size - 1) / block_size;
        add_block_prefix<<<addGrid, block_size>>>(
            d_output, d_blockPrefix, N, elemsPerBlock);

        CUDA_CHECK(cudaFree(d_blockPrefix));
        CUDA_CHECK(cudaFree(d_blockSums));
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--size", 1048576);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int iters      = parseIterations(argc, argv);

    size_t bytes = (size_t)N * sizeof(float);

    // Host allocations
    float* h_input  = (float*)malloc(bytes);
    float* h_output = (float*)malloc(bytes);
    float* h_ref    = (float*)malloc(bytes);

    // Initialize
    srand(42);
    for (int i = 0; i < N; ++i) {
        h_input[i] = (float)(rand() % 10);
    }

    // Device allocations
    float *d_input, *d_output;
    CUDA_CHECK(cudaMalloc(&d_input,  bytes));
    CUDA_CHECK(cudaMalloc(&d_output, bytes));

    CUDA_CHECK(cudaMemcpy(d_input, h_input, bytes, cudaMemcpyHostToDevice));

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("ep", size_str, iters, [&]() {
        gpu_scan(d_input, d_output, N, block_size);
    });

    printCSVRow(r);

    // Verify
    CUDA_CHECK(cudaMemcpy(h_output, d_output, bytes, cudaMemcpyDeviceToHost));
    exclusive_scan_cpu(h_input, h_ref, N);

    double maxErr = 0.0;
    int errCount = 0;
    for (int i = 0; i < N; ++i) {
        double err = fabs((double)h_output[i] - (double)h_ref[i]);
        double denom = fabs((double)h_ref[i]) + 1e-10;
        double relErr = err / denom;
        if (relErr > maxErr) maxErr = relErr;
        if (relErr > 1e-3) errCount++;
    }

    fprintf(stderr, "Max relative error: %e, error count: %d / %d\n",
            maxErr, errCount, N);

    if (errCount == 0) {
        fprintf(stderr, "PASS\n");
    } else {
        fprintf(stderr, "FAIL: %d elements with error > 1e-3\n", errCount);
    }

    // Cleanup
    CUDA_CHECK(cudaFree(d_input));
    CUDA_CHECK(cudaFree(d_output));
    free(h_input);
    free(h_output);
    free(h_ref);

    return (errCount > 0) ? 1 : 0;
}
