// nw.cpp — HIP benchmark for Needleman-Wunsch (NW) sequence alignment
// Kernels copied from amd/benchmarks/rodinia/nw/native/nw.cpp
// Problem size: length=64 (block_size=64)

#include "bench_common.h"

// BLOSUM62 scoring matrix (20x20 amino acids + padding)
static const int blosum62[24][24] = {
    { 4, -1, -2, -2,  0, -1, -1,  0, -2, -1, -1, -1, -1, -2, -1,  1,  0, -3, -2,  0, -2, -1,  0, -4},
    {-1,  5,  0, -2, -3,  1,  0, -2,  0, -3, -2,  2, -1, -3, -2, -1, -1, -3, -2, -3, -1,  0, -1, -4},
    {-2,  0,  6,  1, -3,  0,  0,  0,  1, -3, -3,  0, -2, -3, -2,  1,  0, -4, -2, -3,  3,  0, -1, -4},
    {-2, -2,  1,  6, -3,  0,  2, -1, -1, -3, -4, -1, -3, -3, -1,  0, -1, -4, -3, -3,  4,  1, -1, -4},
    { 0, -3, -3, -3,  9, -3, -4, -3, -3, -1, -1, -3, -1, -2, -3, -1, -1, -2, -2, -1, -3, -3, -2, -4},
    {-1,  1,  0,  0, -3,  5,  2, -2,  0, -3, -2,  1,  0, -3, -1,  0, -1, -2, -1, -2,  0,  3, -1, -4},
    {-1,  0,  0,  2, -4,  2,  5, -2,  0, -3, -3,  1, -2, -3, -1,  0, -1, -3, -2, -2,  1,  4, -1, -4},
    { 0, -2,  0, -1, -3, -2, -2,  6, -2, -4, -4, -2, -3, -3, -2,  0, -2, -2, -3, -3, -1, -2, -1, -4},
    {-2,  0,  1, -1, -3,  0,  0, -2,  8, -3, -3, -1, -2, -1, -2, -1, -2, -2,  2, -3,  0,  0, -1, -4},
    {-1, -3, -3, -3, -1, -3, -3, -4, -3,  4,  2, -3,  1,  0, -3, -2, -1, -3, -1,  3, -3, -3, -1, -4},
    {-1, -2, -3, -4, -1, -2, -3, -4, -3,  2,  4, -2,  2,  0, -3, -2, -1, -2, -1,  1, -4, -3, -1, -4},
    {-1,  2,  0, -1, -3,  1,  1, -2, -1, -3, -2,  5, -1, -3, -1,  0, -1, -3, -2, -2,  0,  1, -1, -4},
    {-1, -1, -2, -3, -1,  0, -2, -3, -2,  1,  2, -1,  5,  0, -2, -1, -1, -1, -1,  1, -3, -1, -1, -4},
    {-2, -3, -3, -3, -2, -3, -3, -3, -1,  0,  0, -3,  0,  6, -4, -2, -2,  1,  3, -1, -3, -3, -1, -4},
    {-1, -2, -2, -1, -3, -1, -1, -2, -2, -3, -3, -1, -2, -4,  7, -1, -1, -4, -3, -2, -2, -1, -2, -4},
    { 1, -1,  1,  0, -1,  0,  0,  0, -1, -2, -2,  0, -1, -2, -1,  4,  1, -3, -2, -2,  0,  0,  0, -4},
    { 0, -1,  0, -1, -1, -1, -1, -2, -2, -1, -1, -1, -1, -2, -1,  1,  5, -2, -2,  0, -1, -1,  0, -4},
    {-3, -3, -4, -4, -2, -2, -3, -2, -2, -3, -2, -3, -1,  1, -4, -3, -2, 11,  2, -3, -4, -3, -2, -4},
    {-2, -2, -2, -3, -2, -1, -2, -3,  2, -1, -1, -2, -1,  3, -3, -2, -2,  2,  7, -1, -3, -2, -1, -4},
    { 0, -3, -3, -3, -1, -2, -2, -3, -3,  3,  1, -2,  1, -1, -2, -2,  0, -3, -1,  4, -3, -2, -1, -4},
    {-2, -1,  3,  4, -3,  0,  1, -1,  0, -3, -4,  0, -3, -3, -2,  0, -1, -4, -3, -3,  4,  1, -1, -4},
    {-1,  0,  0,  1, -3,  3,  4, -2,  0, -3, -3,  1, -1, -3, -1,  0, -1, -3, -2, -2,  1,  4, -1, -4},
    { 0, -1, -1, -1, -2, -1, -1, -1, -1, -1, -1, -1, -1, -1, -2,  0,  0, -2, -1, -1, -1, -1, -1, -4},
    {-4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4,  1}
};

#define SCORE(i, j, block_size) input_itemsets_l[(j) + (i) * ((block_size) + 1)]
#define REF(i, j, block_size) reference_l[(j) + (i) * (block_size)]

__device__ int maximum(int a, int b, int c) {
    int k = (a <= b) ? b : a;
    return (k <= c) ? c : k;
}

extern "C" __global__ void nw_kernel1(
    int* reference_d,
    int* input_itemsets_d,
    int* output_itemsets_d,
    int cols, int penalty,
    int blk, int block_size, int block_width, int worksize,
    int offset_r, int offset_c)
{
    __shared__ int input_itemsets_l[65 * 65];
    __shared__ int reference_l[64 * 64];

    int bx = hipBlockIdx_x;
    int tx = hipThreadIdx_x;

    int base = offset_r * cols + offset_c;
    int b_index_x = bx;
    int b_index_y = blk - 1 - bx;

    int index    = base + cols * block_size * b_index_y + block_size * b_index_x + tx + (cols + 1);
    int index_n  = base + cols * block_size * b_index_y + block_size * b_index_x + tx + (1);
    int index_w  = base + cols * block_size * b_index_y + block_size * b_index_x + (cols);
    int index_nw = base + cols * block_size * b_index_y + block_size * b_index_x;

    if (tx == 0) {
        SCORE(tx, 0, block_size) = input_itemsets_d[index_nw + tx];
    }
    __syncthreads();

    for (int ty = 0; ty < block_size; ty++)
        REF(ty, tx, block_size) = reference_d[index + cols * ty];
    __syncthreads();

    SCORE((tx + 1), 0, block_size) = input_itemsets_d[index_w + cols * tx];
    __syncthreads();

    SCORE(0, (tx + 1), block_size) = input_itemsets_d[index_n];
    __syncthreads();

    for (int m = 0; m < block_size; m++) {
        if (tx <= m) {
            int t_index_x = tx + 1;
            int t_index_y = m - tx + 1;
            SCORE(t_index_y, t_index_x, block_size) =
                maximum(SCORE((t_index_y-1),(t_index_x-1),block_size) + REF((t_index_y-1),(t_index_x-1),block_size),
                        SCORE((t_index_y),(t_index_x-1),block_size) - penalty,
                        SCORE((t_index_y-1),(t_index_x),block_size) - penalty);
        }
        __syncthreads();
    }

    for (int m = block_size - 2; m >= 0; m--) {
        if (tx <= m) {
            int t_index_x = tx + block_size - m;
            int t_index_y = block_size - tx;
            SCORE(t_index_y, t_index_x, block_size) =
                maximum(SCORE((t_index_y-1),(t_index_x-1),block_size) + REF((t_index_y-1),(t_index_x-1),block_size),
                        SCORE((t_index_y),(t_index_x-1),block_size) - penalty,
                        SCORE((t_index_y-1),(t_index_x),block_size) - penalty);
        }
        __syncthreads();
    }

    for (int ty = 0; ty < block_size; ty++)
        input_itemsets_d[index + cols * ty] = SCORE((ty + 1), (tx + 1), block_size);
}

extern "C" __global__ void nw_kernel2(
    int* reference_d,
    int* input_itemsets_d,
    int* output_itemsets_d,
    int cols, int penalty,
    int blk, int block_size, int block_width, int worksize,
    int offset_r, int offset_c)
{
    __shared__ int input_itemsets_l[65 * 65];
    __shared__ int reference_l[64 * 64];

    int bx = hipBlockIdx_x;
    int tx = hipThreadIdx_x;

    int base = offset_r * cols + offset_c;
    int b_index_x = bx + block_width - blk;
    int b_index_y = block_width - bx - 1;

    int index    = base + cols * block_size * b_index_y + block_size * b_index_x + tx + (cols + 1);
    int index_n  = base + cols * block_size * b_index_y + block_size * b_index_x + tx + (1);
    int index_w  = base + cols * block_size * b_index_y + block_size * b_index_x + (cols);
    int index_nw = base + cols * block_size * b_index_y + block_size * b_index_x;

    if (tx == 0) SCORE(tx, 0, block_size) = input_itemsets_d[index_nw];

    for (int ty = 0; ty < block_size; ty++)
        REF(ty, tx, block_size) = reference_d[index + cols * ty];
    __syncthreads();

    SCORE((tx + 1), 0, block_size) = input_itemsets_d[index_w + cols * tx];
    __syncthreads();

    SCORE(0, (tx + 1), block_size) = input_itemsets_d[index_n];
    __syncthreads();

    for (int m = 0; m < block_size; m++) {
        if (tx <= m) {
            int t_index_x = tx + 1;
            int t_index_y = m - tx + 1;
            SCORE(t_index_y, t_index_x, block_size) =
                maximum(SCORE((t_index_y-1),(t_index_x-1),block_size) + REF((t_index_y-1),(t_index_x-1),block_size),
                        SCORE((t_index_y),(t_index_x-1),block_size) - penalty,
                        SCORE((t_index_y-1),(t_index_x),block_size) - penalty);
        }
        __syncthreads();
    }

    for (int m = block_size - 2; m >= 0; m--) {
        if (tx <= m) {
            int t_index_x = tx + block_size - m;
            int t_index_y = block_size - tx;
            SCORE(t_index_y, t_index_x, block_size) =
                maximum(SCORE((t_index_y-1),(t_index_x-1),block_size) + REF((t_index_y-1),(t_index_x-1),block_size),
                        SCORE((t_index_y),(t_index_x-1),block_size) - penalty,
                        SCORE((t_index_y-1),(t_index_x),block_size) - penalty);
        }
        __syncthreads();
    }

    for (int ty = 0; ty < block_size; ty++)
        input_itemsets_d[index + ty * cols] = SCORE((ty + 1), (tx + 1), block_size);
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    // LENGTH must be a multiple of BLOCK_SIZE (64)
    int LENGTH = parseIntParam(argc, argv, "--size", 64);
    const int BLOCK_SIZE = 64;
    const int PENALTY = 10;

    // NW operates on a (LENGTH+1) x (LENGTH+1) matrix
    int cols = LENGTH + 1;
    int rows = LENGTH + 1;
    int total = cols * rows;

    // Host allocations
    std::vector<int> h_input_itemsets(total);
    std::vector<int> h_reference(total);

    // Initialize first row and column with gap penalties
    for (int i = 0; i < cols; i++) {
        h_input_itemsets[i] = -i * PENALTY;
    }
    for (int i = 0; i < rows; i++) {
        h_input_itemsets[i * cols] = -i * PENALTY;
    }

    // Fill reference with BLOSUM62-derived scores
    srand(42);
    for (int i = 1; i < rows; i++) {
        for (int j = 1; j < cols; j++) {
            int a = rand() % 24;
            int b = rand() % 24;
            h_reference[i * cols + j] = blosum62[a][b];
        }
    }

    // Device allocations
    int *d_input_itemsets, *d_reference, *d_output;
    HIP_CHECK(hipMalloc(&d_input_itemsets, total * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_reference, total * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_output, total * sizeof(int))); // unused but kernel expects it

    HIP_CHECK(hipMemcpy(d_input_itemsets, h_input_itemsets.data(), total * sizeof(int), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_reference, h_reference.data(), total * sizeof(int), hipMemcpyHostToDevice));

    int block_width = LENGTH / BLOCK_SIZE;
    int worksize = block_width;

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "length=%d", LENGTH);

    // The NW benchmark iterates over anti-diagonals with multiple kernel launches
    BenchResult r = runBenchmark("nw", problemSize, iterations, [&]() {
        // Reset input_itemsets for each iteration
        HIP_CHECK(hipMemcpy(d_input_itemsets, h_input_itemsets.data(), total * sizeof(int), hipMemcpyHostToDevice));

        // Upper-left triangle passes (kernel1)
        for (int blk = 1; blk <= block_width; blk++) {
            dim3 grid(blk);
            dim3 block(BLOCK_SIZE);
            nw_kernel1<<<grid, block>>>(
                d_reference, d_input_itemsets, d_output,
                cols, PENALTY, blk, BLOCK_SIZE, block_width, worksize,
                0, 0);
        }

        // Lower-right triangle passes (kernel2)
        for (int blk = block_width - 1; blk >= 1; blk--) {
            dim3 grid(blk);
            dim3 block(BLOCK_SIZE);
            nw_kernel2<<<grid, block>>>(
                d_reference, d_input_itemsets, d_output,
                cols, PENALTY, blk, BLOCK_SIZE, block_width, worksize,
                0, 0);
        }
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_input_itemsets));
    HIP_CHECK(hipFree(d_reference));
    HIP_CHECK(hipFree(d_output));

    return 0;
}
