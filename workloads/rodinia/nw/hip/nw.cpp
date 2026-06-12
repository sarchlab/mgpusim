// nw.cpp -- HIP benchmark for Rodinia Needleman-Wunsch
// Sequence alignment using dynamic programming with 2-phase diagonal fill.
// 2 kernels: needle_cuda_shared_1 (upper-left triangle) and
//            needle_cuda_shared_2 (lower-right triangle).

#include "bench_common_hip.h"

#define MATCH_SCORE 1
#define MISMATCH_SCORE -1

#define MAX_BLOCK_SIZE 32

__device__ int blosum_lookup(int a, int b) {
    return (a == b) ? MATCH_SCORE : MISMATCH_SCORE;
}

__global__ void needle_cuda_shared_1(int *d_reference, int *d_matrix,
                                     int cols, int penalty, int block_idx,
                                     int block_size) {
    int bx = hipBlockIdx_x;
    int tx = hipThreadIdx_x;

    extern __shared__ int shared[];
    int *temp = shared;
    int *ref_shared = &shared[(block_size + 1) * (block_size + 1)];

    if (bx > block_idx) return;

    int b_row = block_idx - bx;
    int b_col = bx;

    int begin_row = b_row * block_size + 1;
    int begin_col = b_col * block_size + 1;

    if (tx < block_size) {
        ref_shared[tx] = d_reference[begin_col + tx];
    }
    __syncthreads();

    if (tx == 0) {
        for (int i = 0; i <= block_size; i++) {
            temp[i * (block_size + 1)] =
                d_matrix[(begin_row + i - 1) * cols + (begin_col - 1)];
        }
        for (int j = 0; j <= block_size; j++) {
            temp[j] = d_matrix[(begin_row - 1) * cols + (begin_col + j - 1)];
        }
    }
    __syncthreads();

    for (int diag = 0; diag < 2 * block_size - 1; diag++) {
        int i = (tx <= diag && tx < block_size && (diag - tx) < block_size)
                    ? tx + 1
                    : -1;
        int j = (i > 0) ? diag - tx + 1 : -1;

        if (i > 0 && j > 0 && i <= block_size && j <= block_size) {
            int idx_diag = i * (block_size + 1) + j;
            int idx_up = (i - 1) * (block_size + 1) + j;
            int idx_left = i * (block_size + 1) + (j - 1);
            int idx_upleft = (i - 1) * (block_size + 1) + (j - 1);

            int match = temp[idx_upleft] +
                        blosum_lookup(d_reference[begin_row + i - 1],
                                      ref_shared[j - 1]);
            int del = temp[idx_up] - penalty;
            int ins = temp[idx_left] - penalty;
            int val = match;
            if (del > val) val = del;
            if (ins > val) val = ins;
            temp[idx_diag] = val;
        }
        __syncthreads();
    }

    if (tx < block_size) {
        for (int i = 1; i <= block_size; i++) {
            d_matrix[(begin_row + i - 1) * cols + (begin_col + tx)] =
                temp[i * (block_size + 1) + tx + 1];
        }
    }
}

__global__ void needle_cuda_shared_2(int *d_reference, int *d_matrix,
                                     int cols, int penalty, int block_idx,
                                     int num_blocks, int block_size) {
    int bx = hipBlockIdx_x;
    int tx = hipThreadIdx_x;

    extern __shared__ int shared[];
    int *temp = shared;
    int *ref_shared = &shared[(block_size + 1) * (block_size + 1)];

    int blocks_on_diag = num_blocks - 1 - block_idx;
    if (bx >= blocks_on_diag) return;

    int b_row = num_blocks - 1 - bx;
    int b_col = block_idx + 1 + bx;

    int begin_row = b_row * block_size + 1;
    int begin_col = b_col * block_size + 1;

    if (tx < block_size) {
        ref_shared[tx] = d_reference[begin_col + tx];
    }
    __syncthreads();

    if (tx == 0) {
        for (int i = 0; i <= block_size; i++) {
            temp[i * (block_size + 1)] =
                d_matrix[(begin_row + i - 1) * cols + (begin_col - 1)];
        }
        for (int j = 0; j <= block_size; j++) {
            temp[j] = d_matrix[(begin_row - 1) * cols + (begin_col + j - 1)];
        }
    }
    __syncthreads();

    for (int diag = 0; diag < 2 * block_size - 1; diag++) {
        int i = (tx <= diag && tx < block_size && (diag - tx) < block_size)
                    ? tx + 1
                    : -1;
        int j = (i > 0) ? diag - tx + 1 : -1;

        if (i > 0 && j > 0 && i <= block_size && j <= block_size) {
            int idx_diag = i * (block_size + 1) + j;
            int idx_up = (i - 1) * (block_size + 1) + j;
            int idx_left = i * (block_size + 1) + (j - 1);
            int idx_upleft = (i - 1) * (block_size + 1) + (j - 1);

            int match = temp[idx_upleft] +
                        blosum_lookup(d_reference[begin_row + i - 1],
                                      ref_shared[j - 1]);
            int del = temp[idx_up] - penalty;
            int ins = temp[idx_left] - penalty;
            int val = match;
            if (del > val) val = del;
            if (ins > val) val = ins;
            temp[idx_diag] = val;
        }
        __syncthreads();
    }

    if (tx < block_size) {
        for (int i = 1; i <= block_size; i++) {
            d_matrix[(begin_row + i - 1) * cols + (begin_col + tx)] =
                temp[i * (block_size + 1) + tx + 1];
        }
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int seq_len = parseIntParam(argc, argv, "--sequence_length", 1024);
    int block_size = parseIntParam(argc, argv, "--block_size", 16);
    int penalty = parseIntParam(argc, argv, "--penalty", 10);

    int num_blocks = (seq_len + block_size - 1) / block_size;
    int padded_len = num_blocks * block_size;
    int cols = padded_len + 1;
    int rows = padded_len + 1;
    long matrix_size = (long)rows * cols;

    int *h_reference = (int *)malloc((padded_len + 1) * sizeof(int));
    int *h_matrix = (int *)malloc(matrix_size * sizeof(int));

    srand(42);
    for (int i = 0; i <= padded_len; i++) {
        h_reference[i] = rand() % 10;
    }

    for (int i = 0; i < rows; i++) {
        h_matrix[i * cols] = -i * penalty;
    }
    for (int j = 0; j < cols; j++) {
        h_matrix[j] = -j * penalty;
    }

    int *d_reference, *d_matrix;
    HIP_CHECK(hipMalloc(&d_reference, (padded_len + 1) * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_matrix, matrix_size * sizeof(int)));

    HIP_CHECK(hipMemcpy(d_reference, h_reference,
                        (padded_len + 1) * sizeof(int),
                        hipMemcpyHostToDevice));

    int shared_mem_size = (block_size + 1) * (block_size + 1) * sizeof(int) +
                          block_size * sizeof(int);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", seq_len);

    printCSVHeader();

    BenchResult r = runBenchmark(
        "nw", problemSize, iterations, [&]() {
            HIP_CHECK(hipMemcpy(d_matrix, h_matrix,
                                matrix_size * sizeof(int),
                                hipMemcpyHostToDevice));

            for (int diag = 0; diag < num_blocks; diag++) {
                int blocks_on_diag = diag + 1;
                needle_cuda_shared_1<<<blocks_on_diag, block_size,
                                       shared_mem_size>>>(
                    d_reference, d_matrix, cols, penalty, diag, block_size);
            }

            for (int diag = 0; diag < num_blocks - 1; diag++) {
                int blocks_on_diag = num_blocks - 1 - diag;
                needle_cuda_shared_2<<<blocks_on_diag, block_size,
                                       shared_mem_size>>>(
                    d_reference, d_matrix, cols, penalty, diag, num_blocks,
                    block_size);
            }

            HIP_CHECK(hipDeviceSynchronize());
        });
    printCSVRow(r);

    HIP_CHECK(hipFree(d_reference));
    HIP_CHECK(hipFree(d_matrix));
    free(h_reference);
    free(h_matrix);

    return 0;
}
