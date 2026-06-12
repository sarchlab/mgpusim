// mem_latency.cu -- CUDA benchmark for memory latency via pointer chasing
// Creates a linked list in global memory where each element points to the
// next in a pseudo-random order. Each thread follows the chain for N hops.
// Uses clock64() for device-side timing. Reports ns/hop.

#include "bench_common_cuda.h"

// Pointer-chasing kernel: each thread follows the chain for num_hops steps
__global__ void pointer_chase_kernel(const int *d_array, int array_size,
                                     int num_hops, long long *d_cycles) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    int idx = tid % array_size;

    long long start = clock64();

    for (int i = 0; i < num_hops; i++) {
        idx = d_array[idx];
    }

    long long end = clock64();

    // Store cycles to prevent dead-code elimination and record timing
    d_cycles[tid] = (end - start);

    // Prevent compiler from optimizing away the chain traversal
    if (idx == -999) d_cycles[tid] = idx;
}

// Fisher-Yates shuffle to create a random permutation cycle
void create_chase_array(int *h_array, int N, int stride_type) {
    srand(42);

    if (stride_type == 1) {
        // Sequential stride: each element points to next (wrapping)
        for (int i = 0; i < N; i++) {
            h_array[i] = (i + 1) % N;
        }
    } else {
        // Random: create a random single-cycle permutation
        // Start with identity
        int *perm = (int *)malloc(N * sizeof(int));
        for (int i = 0; i < N; i++) {
            perm[i] = i;
        }
        // Fisher-Yates shuffle
        for (int i = N - 1; i > 0; i--) {
            int j = rand() % (i + 1);
            int tmp = perm[i];
            perm[i] = perm[j];
            perm[j] = tmp;
        }
        // Convert permutation to pointer chain: perm[0]->perm[1]->...->perm[N-1]->perm[0]
        for (int i = 0; i < N - 1; i++) {
            h_array[perm[i]] = perm[i + 1];
        }
        h_array[perm[N - 1]] = perm[0];
        free(perm);
    }
}

int main(int argc, char **argv) {
    int iterations  = parseIterations(argc, argv);
    int array_size  = parseIntParam(argc, argv, "--array_size", 65536);
    int block_size  = parseIntParam(argc, argv, "--block_size", 64);
    int num_hops    = parseIntParam(argc, argv, "--num_hops", 1024);
    int stride_type = parseIntParam(argc, argv, "--stride_type", 2); // 1=sequential, 2=random

    // Allocate and initialize the chase array on host
    int *h_array = (int *)malloc(array_size * sizeof(int));
    create_chase_array(h_array, array_size, stride_type);

    // We launch just 1 block of threads for latency measurement
    int num_threads = block_size;

    // Device allocations
    int *d_array;
    long long *d_cycles;
    CUDA_CHECK(cudaMalloc(&d_array, array_size * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_cycles, num_threads * sizeof(long long)));
    CUDA_CHECK(cudaMemcpy(d_array, h_array, array_size * sizeof(int),
                           cudaMemcpyHostToDevice));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", array_size);

    printCSVHeader();

    dim3 block(block_size);
    dim3 grid(1);

    BenchResult r = runBenchmark("mem_latency_chase", problemSize, iterations, [&]() {
        pointer_chase_kernel<<<grid, block>>>(d_array, array_size,
                                               num_hops, d_cycles);
    });
    printCSVRow(r);

    // Retrieve device-side cycle counts for reporting
    long long *h_cycles = (long long *)malloc(num_threads * sizeof(long long));
    CUDA_CHECK(cudaMemcpy(h_cycles, d_cycles, num_threads * sizeof(long long),
                           cudaMemcpyDeviceToHost));

    // Compute average cycles per hop across all threads
    double total_cycles = 0.0;
    for (int i = 0; i < num_threads; i++) {
        total_cycles += (double)h_cycles[i];
    }
    double avg_cycles_per_hop = total_cycles / (double)(num_threads * num_hops);

    // Get GPU clock rate for ns conversion
    cudaDeviceProp prop;
    CUDA_CHECK(cudaGetDeviceProperties(&prop, 0));
    double clock_rate_ghz = (double)prop.clockRate / 1.0e6; // clockRate is in kHz
    double ns_per_hop = avg_cycles_per_hop / clock_rate_ghz;

    printf("# Avg cycles/hop: %.2f\n", avg_cycles_per_hop);
    printf("# Avg ns/hop: %.2f (at %.2f GHz)\n", ns_per_hop, clock_rate_ghz);
    printf("# Stride type: %s\n", stride_type == 1 ? "sequential" : "random");

    // Cleanup
    CUDA_CHECK(cudaFree(d_array));
    CUDA_CHECK(cudaFree(d_cycles));
    free(h_array);
    free(h_cycles);

    return 0;
}
