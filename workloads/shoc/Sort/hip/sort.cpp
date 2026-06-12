// sort.cpp -- SHOC Sort benchmark (HIP)
// Parallel radix sort on unsigned integers.
// Uses per-bit split approach: for each bit, partition 0s before 1s using scan.

#include "bench_common_hip.h"

#define MAX_BLOCK_SIZE 512

// ---------------------------------------------------------------------------
// Scan kernel (Blelloch-style work-efficient exclusive prefix sum)
// ---------------------------------------------------------------------------
__global__ void scan_block_kernel(unsigned int* __restrict__ output,
                                  const unsigned int* __restrict__ input,
                                  unsigned int* __restrict__ block_sums,
                                  int N) {
    extern __shared__ unsigned int stemp[];

    int tid = threadIdx.x;
    int block_offset = blockIdx.x * (blockDim.x * 2);
    int ai = tid;
    int bi = tid + blockDim.x;

    stemp[ai] = (block_offset + ai < N) ? input[block_offset + ai] : 0;
    stemp[bi] = (block_offset + bi < N) ? input[block_offset + bi] : 0;

    int n = blockDim.x * 2;

    // Up-sweep
    int offset = 1;
    for (int d = n >> 1; d > 0; d >>= 1) {
        __syncthreads();
        if (tid < d) {
            int ai2 = offset * (2 * tid + 1) - 1;
            int bi2 = offset * (2 * tid + 2) - 1;
            stemp[bi2] += stemp[ai2];
        }
        offset <<= 1;
    }

    __syncthreads();
    if (tid == 0) {
        if (block_sums != nullptr) {
            block_sums[blockIdx.x] = stemp[n - 1];
        }
        stemp[n - 1] = 0;
    }

    // Down-sweep
    for (int d = 1; d < n; d <<= 1) {
        offset >>= 1;
        __syncthreads();
        if (tid < d) {
            int ai2 = offset * (2 * tid + 1) - 1;
            int bi2 = offset * (2 * tid + 2) - 1;
            unsigned int t = stemp[ai2];
            stemp[ai2] = stemp[bi2];
            stemp[bi2] += t;
        }
    }

    __syncthreads();
    if (block_offset + ai < N) output[block_offset + ai] = stemp[ai];
    if (block_offset + bi < N) output[block_offset + bi] = stemp[bi];
}

__global__ void add_block_sums_uint_kernel(unsigned int* __restrict__ data,
                                            const unsigned int* __restrict__ block_sums,
                                            int N) {
    int idx = blockIdx.x * (blockDim.x * 2) + threadIdx.x;
    if (blockIdx.x > 0) {
        unsigned int sum = block_sums[blockIdx.x];
        if (idx < N) data[idx] += sum;
        idx += blockDim.x;
        if (idx < N) data[idx] += sum;
    }
}

static void scan_recursive_uint(unsigned int* d_output,
                                 unsigned int* d_input,
                                 int N, int block_size) {
    int elements_per_block = block_size * 2;
    int num_blocks = (N + elements_per_block - 1) / elements_per_block;
    size_t shared_mem = elements_per_block * sizeof(unsigned int);

    unsigned int* d_block_sums = nullptr;
    if (num_blocks > 1) {
        HIP_CHECK(hipMalloc(&d_block_sums, num_blocks * sizeof(unsigned int)));
    }

    hipLaunchKernelGGL(scan_block_kernel, dim3(num_blocks), dim3(block_size),
                       shared_mem, 0, d_output, d_input, d_block_sums, N);

    if (num_blocks > 1) {
        unsigned int* d_scanned;
        HIP_CHECK(hipMalloc(&d_scanned, num_blocks * sizeof(unsigned int)));
        scan_recursive_uint(d_scanned, d_block_sums, num_blocks, block_size);

        hipLaunchKernelGGL(add_block_sums_uint_kernel, dim3(num_blocks),
                           dim3(block_size), 0, 0, d_output, d_scanned, N);

        HIP_CHECK(hipFree(d_scanned));
        HIP_CHECK(hipFree(d_block_sums));
    }
}

// ---------------------------------------------------------------------------
// Radix sort kernels
// ---------------------------------------------------------------------------

// Compute predicate: is bit b of keys[i] == 0?
__global__ void compute_predicates_kernel(unsigned int* __restrict__ predicates,
                                          const unsigned int* __restrict__ keys,
                                          int bit, int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < N) {
        predicates[i] = ((keys[i] >> bit) & 1u) == 0u ? 1u : 0u;
    }
}

// Scatter elements based on scan results
__global__ void scatter_kernel(unsigned int* __restrict__ keys_out,
                               const unsigned int* __restrict__ keys_in,
                               const unsigned int* __restrict__ predicates,
                               const unsigned int* __restrict__ scan_result,
                               unsigned int total_zeros, int N) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i < N) {
        unsigned int key = keys_in[i];
        unsigned int dest;
        if (predicates[i]) {
            // Bit is 0 -- goes to front
            dest = scan_result[i];
        } else {
            // Bit is 1 -- goes after all zeros
            dest = total_zeros + (i - scan_result[i]);
        }
        keys_out[dest] = key;
    }
}

// ---------------------------------------------------------------------------
// CPU reference: standard sort
// ---------------------------------------------------------------------------
static void sort_cpu(unsigned int* arr, int N) {
    // Simple insertion sort for verification (or use stdlib qsort)
    for (int i = 1; i < N; ++i) {
        unsigned int key = arr[i];
        int j = i - 1;
        while (j >= 0 && arr[j] > key) {
            arr[j + 1] = arr[j];
            j--;
        }
        arr[j + 1] = key;
    }
}

static int compare_uint(const void* a, const void* b) {
    unsigned int ua = *(const unsigned int*)a;
    unsigned int ub = *(const unsigned int*)b;
    return (ua > ub) - (ua < ub);
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N              = parseIntParam(argc, argv, "--array_size", 65536);
    int block_size     = parseIntParam(argc, argv, "--block_size", 256);
    int bits_per_pass  = parseIntParam(argc, argv, "--bits_per_pass", 1);
    int iters          = parseIterations(argc, argv);

    if (block_size > MAX_BLOCK_SIZE) block_size = MAX_BLOCK_SIZE;
    // For simplicity, we always do 1-bit radix sort
    (void)bits_per_pass;

    int scan_block = block_size;
    int elements_per_block = scan_block * 2;
    int N_padded = ((N + elements_per_block - 1) / elements_per_block)
                   * elements_per_block;

    size_t bytes = (size_t)N_padded * sizeof(unsigned int);

    // Host allocations
    unsigned int* h_keys = (unsigned int*)calloc(N_padded, sizeof(unsigned int));
    unsigned int* h_ref  = (unsigned int*)malloc((size_t)N * sizeof(unsigned int));

    // Initialize with pseudo-random values
    srand(42);
    for (int i = 0; i < N; ++i) {
        h_keys[i] = (unsigned int)rand();
    }
    memcpy(h_ref, h_keys, (size_t)N * sizeof(unsigned int));

    // Device allocations
    unsigned int *d_keys_in, *d_keys_out;
    unsigned int *d_predicates, *d_scan_result;

    HIP_CHECK(hipMalloc(&d_keys_in, bytes));
    HIP_CHECK(hipMalloc(&d_keys_out, bytes));
    HIP_CHECK(hipMalloc(&d_predicates, bytes));
    HIP_CHECK(hipMalloc(&d_scan_result, bytes));

    int grid = (N + block_size - 1) / block_size;
    int grid_padded = (N_padded + block_size - 1) / block_size;

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("sort", size_str, iters, [&]() {
        // Copy fresh input each iteration
        HIP_CHECK(hipMemcpy(d_keys_in, h_keys, bytes, hipMemcpyHostToDevice));

        for (int bit = 0; bit < 32; ++bit) {
            // Compute predicates (is bit == 0?)
            hipLaunchKernelGGL(compute_predicates_kernel,
                               dim3(grid_padded), dim3(block_size), 0, 0,
                               d_predicates, d_keys_in, bit, N_padded);

            // Exclusive scan of predicates
            scan_recursive_uint(d_scan_result, d_predicates, N_padded,
                               scan_block);

            // Get total number of zeros (scan_result[N-1] + predicates[N-1])
            unsigned int last_scan, last_pred;
            HIP_CHECK(hipMemcpy(&last_scan, d_scan_result + N - 1,
                                sizeof(unsigned int), hipMemcpyDeviceToHost));
            HIP_CHECK(hipMemcpy(&last_pred, d_predicates + N - 1,
                                sizeof(unsigned int), hipMemcpyDeviceToHost));
            unsigned int total_zeros = last_scan + last_pred;

            // Scatter
            hipLaunchKernelGGL(scatter_kernel,
                               dim3(grid), dim3(block_size), 0, 0,
                               d_keys_out, d_keys_in, d_predicates,
                               d_scan_result, total_zeros, N);

            // Swap buffers
            unsigned int* temp = d_keys_in;
            d_keys_in = d_keys_out;
            d_keys_out = temp;
        }
    });

    printCSVRow(r);

    // Verify: run one more time
    HIP_CHECK(hipMemcpy(d_keys_in, h_keys, bytes, hipMemcpyHostToDevice));
    for (int bit = 0; bit < 32; ++bit) {
        hipLaunchKernelGGL(compute_predicates_kernel,
                           dim3(grid_padded), dim3(block_size), 0, 0,
                           d_predicates, d_keys_in, bit, N_padded);
        scan_recursive_uint(d_scan_result, d_predicates, N_padded, scan_block);

        unsigned int last_scan, last_pred;
        HIP_CHECK(hipMemcpy(&last_scan, d_scan_result + N - 1,
                            sizeof(unsigned int), hipMemcpyDeviceToHost));
        HIP_CHECK(hipMemcpy(&last_pred, d_predicates + N - 1,
                            sizeof(unsigned int), hipMemcpyDeviceToHost));
        unsigned int total_zeros = last_scan + last_pred;

        hipLaunchKernelGGL(scatter_kernel,
                           dim3(grid), dim3(block_size), 0, 0,
                           d_keys_out, d_keys_in, d_predicates,
                           d_scan_result, total_zeros, N);

        unsigned int* temp = d_keys_in;
        d_keys_in = d_keys_out;
        d_keys_out = temp;
    }
    HIP_CHECK(hipDeviceSynchronize());

    unsigned int* h_result = (unsigned int*)malloc((size_t)N * sizeof(unsigned int));
    HIP_CHECK(hipMemcpy(h_result, d_keys_in, (size_t)N * sizeof(unsigned int),
                        hipMemcpyDeviceToHost));

    // Sort reference on CPU
    qsort(h_ref, N, sizeof(unsigned int), compare_uint);

    int errors = 0;
    for (int i = 0; i < N; ++i) {
        if (h_result[i] != h_ref[i]) {
            if (errors < 10) {
                fprintf(stderr, "Mismatch at %d: got %u, expected %u\n",
                        i, h_result[i], h_ref[i]);
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
    HIP_CHECK(hipFree(d_keys_in));
    HIP_CHECK(hipFree(d_keys_out));
    HIP_CHECK(hipFree(d_predicates));
    HIP_CHECK(hipFree(d_scan_result));
    free(h_keys);
    free(h_ref);
    free(h_result);

    return (errors > 0) ? 1 : 0;
}
