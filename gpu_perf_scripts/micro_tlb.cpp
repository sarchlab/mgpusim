// micro_tlb.cpp — TLB miss cost microbenchmark for AMD MI300A (gfx942)
//
// Measures TLB miss penalties by accessing memory with page-sized strides
// that exceed TLB coverage, forcing TLB misses at various levels.
//
// MI300A TLB parameters (from simulator builder.go):
//   - Page size: 4 KB  (log2PageSize = 12)
//   - L1 TLB: 4 sets × 64 ways = 256 entries → covers 256 × 4KB = 1 MB
//   - L2 TLB: 64 sets × 64 ways = 4096 entries → covers 4096 × 4KB = 16 MB
//
// Tests:
//   1. TLB-hit baseline   — access fewer pages than L1 TLB can hold (64 pages)
//   2. L1 TLB miss        — access more pages than L1 TLB (>256), fits in L2 TLB
//   3. L2 TLB miss        — access more pages than L2 TLB (>4096)
//   4. Huge-stride test   — stride of 2 MB (512 pages) to stress L2 TLB
//
// Each test uses a single GPU thread that performs pointer chasing across
// pages to prevent prefetching.
//
// Build:  hipcc -O2 micro_tlb.cpp -o micro_tlb
// Run:    ./micro_tlb [--iterations N]
//
// Output CSV: test_name,num_pages,stride_kb,iterations,avg_ns_per_access,estimated_cycles

#include "bench_common.h"
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>
#include <algorithm>
#include <numeric>
#include <chrono>

// GPU frequency in GHz (1700 MHz = 1.7 GHz)
static const double GPU_FREQ_GHZ = 1.7;

// ---------------------------------------------------------------------------
// Kernel: strided pointer chase
//
// d_indices holds the next-page index at each page's base offset.
// The kernel chases through pages: page_idx = d_data[page_idx * stride_elems]
// for num_chases hops.  'd_data' is an array of unsigned ints where
// element at index (page_idx * stride_elems) holds the next page index
// multiplied by stride_elems — i.e., the direct array index of the next hop.
// ---------------------------------------------------------------------------
__global__ void tlb_chase_kernel(const unsigned int* __restrict__ d_data,
                                 unsigned int* __restrict__ d_result,
                                 unsigned int start_idx,
                                 unsigned int num_chases) {
    unsigned int idx = start_idx;
    for (unsigned int i = 0; i < num_chases; ++i) {
        idx = d_data[idx];
    }
    *d_result = idx;
}

// ---------------------------------------------------------------------------
// Build a random page-chase pattern.
//
// We allocate `num_pages` worth of memory.  At offset
// page_i * stride_elems we store the array index of the next page in
// the chain (i.e., next_page * stride_elems).  This creates a random
// cyclic permutation over pages.
// ---------------------------------------------------------------------------
static void build_page_chain(std::vector<unsigned int>& data,
                             unsigned int num_pages,
                             unsigned int stride_elems) {
    size_t total_elems = (size_t)num_pages * stride_elems;
    data.assign(total_elems, 0u);

    // Random cyclic permutation of pages
    std::vector<unsigned int> order(num_pages);
    std::iota(order.begin(), order.end(), 0u);

    unsigned int seed = 12345u;
    auto rng = [&seed](unsigned int n) -> unsigned int {
        seed = seed * 1664525u + 1013904223u;
        return seed % n;
    };

    for (unsigned int i = num_pages - 1; i > 0; --i) {
        unsigned int j = rng(i + 1);
        std::swap(order[i], order[j]);
    }

    // Wire the chain
    for (unsigned int k = 0; k < num_pages - 1; ++k) {
        data[(size_t)order[k] * stride_elems] =
            order[k + 1] * stride_elems;
    }
    data[(size_t)order[num_pages - 1] * stride_elems] =
        order[0] * stride_elems;
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
// Run one TLB test configuration and print a CSV row
// ---------------------------------------------------------------------------
static void run_tlb_test(const char* test_name,
                         unsigned int num_pages,
                         unsigned int stride_kb,
                         unsigned int num_chases,
                         int iters) {
    unsigned int stride_bytes = stride_kb * 1024u;
    unsigned int stride_elems = stride_bytes / sizeof(unsigned int);

    // Build host-side chain
    std::vector<unsigned int> h_data;
    build_page_chain(h_data, num_pages, stride_elems);

    size_t total_bytes = h_data.size() * sizeof(unsigned int);

    // Upload to device
    unsigned int* d_data = nullptr;
    unsigned int* d_result = nullptr;
    HIP_CHECK(hipMalloc(&d_data, total_bytes));
    HIP_CHECK(hipMalloc(&d_result, sizeof(unsigned int)));
    HIP_CHECK(hipMemcpy(d_data, h_data.data(), total_bytes,
                         hipMemcpyHostToDevice));
    HIP_CHECK(hipDeviceSynchronize());

    // Starting index: first page in the permutation (index 0's content)
    unsigned int start_idx = 0;

    // Warmup
    hipLaunchKernelGGL(tlb_chase_kernel,
                       dim3(1), dim3(1), 0, 0,
                       d_data, d_result, start_idx, num_chases);
    HIP_CHECK(hipDeviceSynchronize());

    // Timed iterations
    double total_ms = 0.0;
    for (int it = 0; it < iters; ++it) {
        double t0 = wall_time_ms();
        hipLaunchKernelGGL(tlb_chase_kernel,
                           dim3(1), dim3(1), 0, 0,
                           d_data, d_result, start_idx, num_chases);
        HIP_CHECK(hipDeviceSynchronize());
        double t1 = wall_time_ms();
        total_ms += (t1 - t0);
    }

    double avg_total_ms = total_ms / iters;
    double avg_ns_per_access =
        (avg_total_ms * 1.0e6) / static_cast<double>(num_chases);
    double estimated_cycles = avg_ns_per_access * GPU_FREQ_GHZ;

    printf("%s,%u,%u,%d,%.2f,%.1f\n",
           test_name, num_pages, stride_kb, iters,
           avg_ns_per_access, estimated_cycles);

    HIP_CHECK(hipFree(d_data));
    HIP_CHECK(hipFree(d_result));
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int iters = parseIterations(argc, argv);

    // Number of chases per measurement
    const unsigned int NUM_CHASES = 500000;

    printf("# TLB Miss Cost Microbenchmark (MI300A gfx942)\n");
    printf("# Page size: 4 KB\n");
    printf("# L1 TLB: 256 entries (1 MB coverage)\n");
    printf("# L2 TLB: 4096 entries (16 MB coverage)\n");
    printf("# Chases per measurement: %u\n", NUM_CHASES);
    printf("# Measurement iterations: %d\n", iters);
    printf("# GPU freq assumption: %.0f MHz\n", GPU_FREQ_GHZ * 1000.0);
    printf("#\n");
    printf("test_name,num_pages,stride_kb,iterations,"
           "avg_ns_per_access,estimated_cycles\n");

    // ------------------------------------------------------------------
    // Test 1: TLB-hit baseline — 64 pages with 4 KB stride
    //         64 pages × 4 KB = 256 KB ≪ L1 TLB coverage (1 MB)
    // ------------------------------------------------------------------
    run_tlb_test("tlb_hit_baseline", 64, 4, NUM_CHASES, iters);

    // ------------------------------------------------------------------
    // Test 2: Near L1 TLB capacity — 192 pages (768 KB < 1 MB)
    // ------------------------------------------------------------------
    run_tlb_test("tlb_l1_fit", 192, 4, NUM_CHASES, iters);

    // ------------------------------------------------------------------
    // Test 3: L1 TLB miss — 512 pages × 4 KB = 2 MB > 1 MB L1 TLB
    //         Should still fit in L2 TLB (4096 entries)
    // ------------------------------------------------------------------
    run_tlb_test("tlb_l1_miss", 512, 4, NUM_CHASES, iters);

    // ------------------------------------------------------------------
    // Test 4: More L1 TLB pressure — 1024 pages × 4 KB = 4 MB
    // ------------------------------------------------------------------
    run_tlb_test("tlb_l1_miss_1024", 1024, 4, NUM_CHASES, iters);

    // ------------------------------------------------------------------
    // Test 5: Near L2 TLB capacity — 2048 pages × 4 KB = 8 MB
    // ------------------------------------------------------------------
    run_tlb_test("tlb_l2_fit", 2048, 4, NUM_CHASES, iters);

    // ------------------------------------------------------------------
    // Test 6: L2 TLB miss — 8192 pages × 4 KB = 32 MB > 16 MB L2 TLB
    // ------------------------------------------------------------------
    run_tlb_test("tlb_l2_miss", 8192, 4, NUM_CHASES, iters);

    // ------------------------------------------------------------------
    // Test 7: Heavy L2 TLB miss — 16384 pages × 4 KB = 64 MB
    // ------------------------------------------------------------------
    run_tlb_test("tlb_l2_miss_heavy", 16384, 4, NUM_CHASES, iters);

    // ------------------------------------------------------------------
    // Test 8: Huge stride — 2 MB stride (512 pages apart), 128 accesses
    //         Each access touches a different 2 MB region, stressing TLB
    //         Total footprint: 128 × 2 MB = 256 MB
    // ------------------------------------------------------------------
    run_tlb_test("huge_stride_2mb", 128, 2048, NUM_CHASES, iters);

    // ------------------------------------------------------------------
    // Test 9: Huge stride — 2 MB stride, 512 accesses (1 GB footprint)
    // ------------------------------------------------------------------
    run_tlb_test("huge_stride_2mb_512", 512, 2048, NUM_CHASES, iters);

    printf("#\n");
    printf("# Done.\n");
    return 0;
}
