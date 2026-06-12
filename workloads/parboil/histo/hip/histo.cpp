// histo.cpp -- HIP benchmark for Parboil Histogram Computation
// Kernel: Parallel histogram using atomicAdd -- each thread reads an input
// value and increments the corresponding bin, with optional saturation capping.

#include "bench_common_hip.h"

// Kernel: each thread processes one element, atomically incrementing the bin
__global__ void histogram(unsigned int *input, unsigned int *bins,
                          int num_elements, int num_bins,
                          unsigned int saturation) {
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (gid >= num_elements) return;

    unsigned int bin = input[gid] % num_bins;
    unsigned int old = atomicAdd(&bins[bin], 1u);

    // Saturation: cap the bin count at the saturation value
    if (saturation > 0 && old >= saturation) {
        atomicSub(&bins[bin], 1u);
    }
}

// Kernel: clear histogram bins to zero
__global__ void clear_bins(unsigned int *bins, int num_bins) {
    int gid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (gid < num_bins) {
        bins[gid] = 0;
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_elements = parseIntParam(argc, argv, "--num_elements", 1048576);
    int num_bins = parseIntParam(argc, argv, "--num_bins", 256);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    unsigned int saturation =
        (unsigned int)parseIntParam(argc, argv, "--saturation", 255);

    // Allocate host memory
    unsigned int *h_input =
        (unsigned int *)malloc(num_elements * sizeof(unsigned int));
    unsigned int *h_bins =
        (unsigned int *)malloc(num_bins * sizeof(unsigned int));

    // Initialize input data programmatically
    srand(42);
    for (int i = 0; i < num_elements; i++) {
        h_input[i] = (unsigned int)(rand() % num_bins);
    }
    for (int i = 0; i < num_bins; i++) {
        h_bins[i] = 0;
    }

    // Device allocations
    unsigned int *d_input, *d_bins;
    HIP_CHECK(hipMalloc(&d_input, num_elements * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_bins, num_bins * sizeof(unsigned int)));

    HIP_CHECK(hipMemcpy(d_input, h_input,
                        num_elements * sizeof(unsigned int),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_bins, h_bins, num_bins * sizeof(unsigned int),
                        hipMemcpyHostToDevice));

    // Kernel launch configs
    dim3 block_hist(block_size);
    dim3 grid_hist((num_elements + block_size - 1) / block_size);

    dim3 block_clear(block_size);
    dim3 grid_clear((num_bins + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_elements);

    printCSVHeader();

    BenchResult r1 = runBenchmark(
        "histogram", problemSize, iterations, [&]() {
            // Clear bins before each histogram computation
            clear_bins<<<grid_clear, block_clear>>>(d_bins, num_bins);
            histogram<<<grid_hist, block_hist>>>(d_input, d_bins, num_elements,
                                                  num_bins, saturation);
        });
    printCSVRow(r1);

    // Cleanup
    HIP_CHECK(hipFree(d_input));
    HIP_CHECK(hipFree(d_bins));
    free(h_input);
    free(h_bins);

    return 0;
}
