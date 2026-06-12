// hist.cu -- Heteromark Histogram benchmark (CUDA)
// Compute histogram of input data using shared memory optimization.

#include "bench_common_cuda.h"

#define DEFAULT_SIZE    1048576
#define DEFAULT_BINS    256
#define DEFAULT_BLOCK   256

// ---------------------------------------------------------------------------
// Kernel: Histogram with shared memory per-block accumulation
// ---------------------------------------------------------------------------
__global__ void histogram_kernel(const unsigned int* __restrict__ data,
                                 unsigned int* __restrict__ hist,
                                 int N,
                                 int numBins) {
    extern __shared__ unsigned int localHist[];

    int tid = threadIdx.x;
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    int stride = blockDim.x * gridDim.x;

    // Zero shared memory histogram
    for (int i = tid; i < numBins; i += blockDim.x) {
        localHist[i] = 0;
    }
    __syncthreads();

    // Accumulate into shared memory histogram
    for (int i = gid; i < N; i += stride) {
        atomicAdd(&localHist[data[i]], 1);
    }
    __syncthreads();

    // Add shared histogram to global histogram
    for (int i = tid; i < numBins; i += blockDim.x) {
        if (localHist[i] > 0) {
            atomicAdd(&hist[i], localHist[i]);
        }
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--size", DEFAULT_SIZE);
    int numBins    = parseIntParam(argc, argv, "--num_bins", DEFAULT_BINS);
    int block_size = parseIntParam(argc, argv, "--block_size", DEFAULT_BLOCK);
    int iters      = parseIterations(argc, argv);

    size_t dataBytes = (size_t)N * sizeof(unsigned int);
    size_t histBytes = (size_t)numBins * sizeof(unsigned int);

    // Host allocations
    unsigned int* h_data = (unsigned int*)malloc(dataBytes);
    unsigned int* h_hist = (unsigned int*)malloc(histBytes);

    // Initialize random data in range [0, numBins-1]
    srand(42);
    for (int i = 0; i < N; ++i) {
        h_data[i] = (unsigned int)(rand() % numBins);
    }

    // Device allocations
    unsigned int *d_data, *d_hist;
    CUDA_CHECK(cudaMalloc(&d_data, dataBytes));
    CUDA_CHECK(cudaMalloc(&d_hist, histBytes));

    CUDA_CHECK(cudaMemcpy(d_data, h_data, dataBytes, cudaMemcpyHostToDevice));

    int grid = (N + block_size - 1) / block_size;
    // Cap grid size to avoid too many blocks
    if (grid > 1024) grid = 1024;

    size_t sharedMemSize = (size_t)numBins * sizeof(unsigned int);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("hist", problemSize, iters, [&]() {
        // Clear histogram each iteration
        CUDA_CHECK(cudaMemset(d_hist, 0, histBytes));
        histogram_kernel<<<grid, block_size, sharedMemSize>>>(
            d_data, d_hist, N, numBins);
    });

    printCSVRow(r);

    // Verification: compute CPU histogram and compare
    CUDA_CHECK(cudaMemset(d_hist, 0, histBytes));
    histogram_kernel<<<grid, block_size, sharedMemSize>>>(
        d_data, d_hist, N, numBins);
    CUDA_CHECK(cudaDeviceSynchronize());
    CUDA_CHECK(cudaMemcpy(h_hist, d_hist, histBytes, cudaMemcpyDeviceToHost));

    unsigned int* ref_hist = (unsigned int*)calloc(numBins, sizeof(unsigned int));
    for (int i = 0; i < N; ++i) {
        ref_hist[h_data[i]]++;
    }

    int errors = 0;
    for (int i = 0; i < numBins; ++i) {
        if (h_hist[i] != ref_hist[i]) {
            if (errors < 5) {
                fprintf(stderr, "Mismatch at bin %d: GPU=%u CPU=%u\n",
                        i, h_hist[i], ref_hist[i]);
            }
            errors++;
        }
    }

    if (errors == 0) {
        fprintf(stderr, "PASS\n");
    } else {
        fprintf(stderr, "FAIL: %d bins mismatched\n", errors);
    }

    // Cleanup
    CUDA_CHECK(cudaFree(d_data));
    CUDA_CHECK(cudaFree(d_hist));
    free(h_data);
    free(h_hist);
    free(ref_hist);

    return 0;
}
