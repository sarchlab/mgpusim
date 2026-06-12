// scan.cu -- SHOC Scan benchmark (CUDA)
// Parallel exclusive prefix sum (Blelloch-style work-efficient scan).
// Supports arrays larger than a single block via block-level scan + fixup.

#include "bench_common_cuda.h"

#define MAX_BLOCK_SIZE 512

// ---------------------------------------------------------------------------
// Shared-memory work-efficient scan (exclusive) for one block
// ---------------------------------------------------------------------------
__global__ void scan_block_kernel(float* __restrict__ output,
                                  const float* __restrict__ input,
                                  float* __restrict__ block_sums,
                                  int N) {
    extern __shared__ float temp[];

    int tid = threadIdx.x;
    int block_offset = blockIdx.x * (blockDim.x * 2);
    int ai = tid;
    int bi = tid + blockDim.x;

    // Load input into shared memory
    temp[ai] = (block_offset + ai < N) ? input[block_offset + ai] : 0.0f;
    temp[bi] = (block_offset + bi < N) ? input[block_offset + bi] : 0.0f;

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
        if (block_sums != nullptr) {
            block_sums[blockIdx.x] = temp[n - 1];
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
    if (block_offset + ai < N) output[block_offset + ai] = temp[ai];
    if (block_offset + bi < N) output[block_offset + bi] = temp[bi];
}

// ---------------------------------------------------------------------------
// Add block sums back to scanned blocks
// ---------------------------------------------------------------------------
__global__ void add_block_sums_kernel(float* __restrict__ data,
                                      const float* __restrict__ block_sums,
                                      int N) {
    int idx = blockIdx.x * (blockDim.x * 2) + threadIdx.x;
    if (blockIdx.x > 0) {
        float sum = block_sums[blockIdx.x];
        if (idx < N) data[idx] += sum;
        idx += blockDim.x;
        if (idx < N) data[idx] += sum;
    }
}

// ---------------------------------------------------------------------------
// Recursive scan for large arrays
// ---------------------------------------------------------------------------
static void scan_recursive(float* d_output, float* d_input, int N,
                            int block_size) {
    int elements_per_block = block_size * 2;
    int num_blocks = (N + elements_per_block - 1) / elements_per_block;
    size_t shared_mem = elements_per_block * sizeof(float);

    float* d_block_sums = nullptr;
    if (num_blocks > 1) {
        CUDA_CHECK(cudaMalloc(&d_block_sums, num_blocks * sizeof(float)));
    }

    scan_block_kernel<<<num_blocks, block_size, shared_mem>>>(
        d_output, d_input, d_block_sums, N);

    if (num_blocks > 1) {
        // Recursively scan block sums
        float* d_scanned_block_sums;
        CUDA_CHECK(cudaMalloc(&d_scanned_block_sums,
                               num_blocks * sizeof(float)));
        scan_recursive(d_scanned_block_sums, d_block_sums, num_blocks,
                       block_size);

        // Add scanned block sums back
        add_block_sums_kernel<<<num_blocks, block_size>>>(
            d_output, d_scanned_block_sums, N);

        CUDA_CHECK(cudaFree(d_scanned_block_sums));
        CUDA_CHECK(cudaFree(d_block_sums));
    }
}

// ---------------------------------------------------------------------------
// CPU reference: exclusive prefix sum
// ---------------------------------------------------------------------------
static void scan_cpu(float* output, const float* input, int N) {
    output[0] = 0.0f;
    for (int i = 1; i < N; ++i) {
        output[i] = output[i - 1] + input[i - 1];
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--array_size", 1048576);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int iters      = parseIterations(argc, argv);

    // block_size must be power of 2 and <= MAX_BLOCK_SIZE
    if (block_size > MAX_BLOCK_SIZE) block_size = MAX_BLOCK_SIZE;

    // Round N up to next multiple of elements_per_block for simplicity
    int elements_per_block = block_size * 2;
    int N_padded = ((N + elements_per_block - 1) / elements_per_block)
                   * elements_per_block;

    size_t bytes = (size_t)N_padded * sizeof(float);

    // Host allocations
    float* h_input  = (float*)calloc(N_padded, sizeof(float));
    float* h_output = (float*)malloc(bytes);
    float* h_ref    = (float*)malloc((size_t)N * sizeof(float));

    // Initialize with random-ish values
    for (int i = 0; i < N; ++i) {
        h_input[i] = (float)(i % 7) + 1.0f;
    }
    // Padding is already zero from calloc

    // Device allocations
    float *d_input, *d_output;
    CUDA_CHECK(cudaMalloc(&d_input, bytes));
    CUDA_CHECK(cudaMalloc(&d_output, bytes));

    CUDA_CHECK(cudaMemcpy(d_input, h_input, bytes, cudaMemcpyHostToDevice));

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("scan", size_str, iters, [&]() {
        CUDA_CHECK(cudaMemcpy(d_output, d_input, bytes,
                              cudaMemcpyDeviceToDevice));
        scan_recursive(d_output, d_input, N_padded, block_size);
    });

    printCSVRow(r);

    // Verify
    CUDA_CHECK(cudaMemcpy(d_output, d_input, bytes, cudaMemcpyDeviceToDevice));
    scan_recursive(d_output, d_input, N_padded, block_size);
    CUDA_CHECK(cudaDeviceSynchronize());
    CUDA_CHECK(cudaMemcpy(h_output, d_output, (size_t)N * sizeof(float),
                          cudaMemcpyDeviceToHost));

    scan_cpu(h_ref, h_input, N);

    int errors = 0;
    for (int i = 0; i < N; ++i) {
        float diff = fabsf(h_output[i] - h_ref[i]);
        float tol = 1e-3f * fabsf(h_ref[i]) + 1e-5f;
        if (diff > tol) {
            if (errors < 10) {
                fprintf(stderr, "Mismatch at %d: got %f, expected %f\n",
                        i, h_output[i], h_ref[i]);
            }
            errors++;
        }
    }
    if (errors > 0) {
        fprintf(stderr, "FAIL: %d errors out of %d\n", errors, N);
    } else {
        fprintf(stderr, "PASS\n");
    }

    // Cleanup
    CUDA_CHECK(cudaFree(d_input));
    CUDA_CHECK(cudaFree(d_output));
    free(h_input);
    free(h_output);
    free(h_ref);

    return (errors > 0) ? 1 : 0;
}
