// micro_cache_latency.cpp — Pointer-chasing cache latency microbenchmark
//                          for AMD MI300A (gfx942)
//
// Measures L1, L2, and DRAM access latency using a classic pointer-chasing
// technique.  A single GPU thread traverses a linked list embedded in an
// array of different sizes:
//   - Small arrays (8–16 KB) fit in L1 cache  → measure L1 latency
//   - Medium arrays (256 KB – 8 MB) fit in L2  → measure L2 latency
//   - Large arrays (64 MB+)  exceed L2         → measure DRAM latency
//
// The pointer chain is randomly shuffled so that hardware prefetchers cannot
// hide the latency.
//
// Build:  hipcc -O2 micro_cache_latency.cpp -o micro_cache_latency
// Run:    ./micro_cache_latency [--iterations N]
//
// Output CSV: level,array_size_kb,iterations,avg_ns,estimated_cycles

#include "bench_common.h"
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>
#include <algorithm>
#include <numeric>
#include <chrono>

// GPU frequency in GHz (1700 MHz = 1.7 GHz) for cycle estimation
static const double GPU_FREQ_GHZ = 1.7;

// ---------------------------------------------------------------------------
// Kernel: single-thread pointer chase
//
// d_arr is the linked list: d_arr[i] holds the index of the next element.
// The kernel follows the chain for 'num_chases' hops and writes the final
// index to *d_result (to prevent the compiler from optimizing away the loop).
// ---------------------------------------------------------------------------
__global__ void pointer_chase_kernel(const unsigned int* __restrict__ d_arr,
                                     unsigned int* __restrict__ d_result,
                                     unsigned int start,
                                     unsigned int num_chases) {
    unsigned int idx = start;
    for (unsigned int i = 0; i < num_chases; ++i) {
        idx = d_arr[idx];
    }
    *d_result = idx;
}

// ---------------------------------------------------------------------------
// Build a random pointer chain of length `count` (a random cyclic permutation
// so that every element is visited exactly once per cycle).
// ---------------------------------------------------------------------------
static void build_random_chain(std::vector<unsigned int>& chain,
                               unsigned int count) {
    chain.resize(count);

    // Create a random permutation using Fisher-Yates
    std::vector<unsigned int> order(count);
    std::iota(order.begin(), order.end(), 0u);

    // Simple seeded LCG for reproducibility
    unsigned int seed = 42u;
    auto rng = [&seed](unsigned int n) -> unsigned int {
        seed = seed * 1664525u + 1013904223u;
        return seed % n;
    };

    for (unsigned int i = count - 1; i > 0; --i) {
        unsigned int j = rng(i + 1);
        std::swap(order[i], order[j]);
    }

    // Convert permutation to linked list: order[k] -> order[k+1]
    for (unsigned int k = 0; k < count - 1; ++k) {
        chain[order[k]] = order[k + 1];
    }
    chain[order[count - 1]] = order[0]; // close the cycle
}

// ---------------------------------------------------------------------------
// Wall-clock helper
// ---------------------------------------------------------------------------
static double wall_time_ms() {
    auto now = std::chrono::steady_clock::now();
    return std::chrono::duration<double, std::milli>(
               now.time_since_epoch())
        .count();
}

// ---------------------------------------------------------------------------
// Classify cache level based on array size
// ---------------------------------------------------------------------------
static const char* classify_level(size_t size_kb) {
    if (size_kb <= 16)
        return "L1";
    else if (size_kb <= 8192)
        return "L2";
    else
        return "DRAM";
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int iters = parseIterations(argc, argv);

    // Number of pointer chases per measurement — enough to amortize launch
    // overhead but not so many that the kernel runs excessively long.
    const unsigned int NUM_CHASES = 1000000;

    // Array sizes to sweep (in KB)
    const size_t sizes_kb[] = {
        8,        // ~L1
        16,       // L1 boundary
        32,       // small L2
        64,       // L2
        128,      // L2
        256,      // L2
        512,      // L2
        1024,     // L2
        2048,     // L2
        4096,     // L2
        8192,     // L2 boundary
        16384,    // DRAM
        32768,    // DRAM
        65536,    // DRAM (64 MB)
        131072    // DRAM (128 MB)
    };
    const int num_sizes = sizeof(sizes_kb) / sizeof(sizes_kb[0]);

    printf("# Cache Latency Microbenchmark — Pointer Chasing (MI300A gfx942)\n");
    printf("# Chases per measurement: %u\n", NUM_CHASES);
    printf("# Measurement iterations: %d\n", iters);
    printf("# GPU freq assumption: %.0f MHz\n", GPU_FREQ_GHZ * 1000.0);
    printf("#\n");
    printf("level,array_size_kb,iterations,avg_ns,estimated_cycles\n");

    unsigned int* d_result = nullptr;
    HIP_CHECK(hipMalloc(&d_result, sizeof(unsigned int)));

    for (int si = 0; si < num_sizes; si++) {
        size_t size_bytes = sizes_kb[si] * 1024ULL;
        // Number of unsigned-int elements
        unsigned int count = static_cast<unsigned int>(
            size_bytes / sizeof(unsigned int));
        if (count < 2) continue;

        // Build random chain on host
        std::vector<unsigned int> h_chain;
        build_random_chain(h_chain, count);

        // Upload to device
        unsigned int* d_arr = nullptr;
        HIP_CHECK(hipMalloc(&d_arr, size_bytes));
        HIP_CHECK(hipMemcpy(d_arr, h_chain.data(), size_bytes,
                             hipMemcpyHostToDevice));
        HIP_CHECK(hipDeviceSynchronize());

        // Warmup
        hipLaunchKernelGGL(pointer_chase_kernel,
                           dim3(1), dim3(1), 0, 0,
                           d_arr, d_result, 0u, NUM_CHASES);
        HIP_CHECK(hipDeviceSynchronize());

        // Timed iterations
        double total_ms = 0.0;
        for (int it = 0; it < iters; ++it) {
            double t0 = wall_time_ms();
            hipLaunchKernelGGL(pointer_chase_kernel,
                               dim3(1), dim3(1), 0, 0,
                               d_arr, d_result, 0u, NUM_CHASES);
            HIP_CHECK(hipDeviceSynchronize());
            double t1 = wall_time_ms();
            total_ms += (t1 - t0);
        }

        double avg_total_ms = total_ms / iters;
        // Subtract approximate kernel-launch overhead (~5 µs) is negligible
        // compared to 1M chases, so we skip that correction.
        double avg_ns_per_access =
            (avg_total_ms * 1.0e6) / static_cast<double>(NUM_CHASES);
        double estimated_cycles = avg_ns_per_access * GPU_FREQ_GHZ;

        const char* level = classify_level(sizes_kb[si]);

        printf("%s,%zu,%d,%.2f,%.1f\n",
               level, sizes_kb[si], iters,
               avg_ns_per_access, estimated_cycles);

        HIP_CHECK(hipFree(d_arr));
    }

    HIP_CHECK(hipFree(d_result));

    printf("#\n");
    printf("# Done.\n");
    return 0;
}
