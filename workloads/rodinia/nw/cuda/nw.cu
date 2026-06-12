// nw.cu -- CUDA benchmark for Rodinia Needleman-Wunsch
// Sequence alignment using dynamic programming with 2-phase diagonal fill.
// 2 kernels: needle_cuda_shared_1 (upper-left triangle) and
//            needle_cuda_shared_2 (lower-right triangle).

#include "bench_common_cuda.h"

#define MATCH_SCORE 1
#define MISMATCH_SCORE -1

// Maximum block size for shared memory allocation
#define MAX_BLOCK_SIZE 32

__device__ int blosum_lookup(int a, int b) {
    return (a == b) ? MATCH_SCORE : MISMATCH_SCORE;
}

__global__ void needle_cuda_shared_1(int *d_reference, int *d_matrix,
                                     int cols, int penalty, int block_idx,
                                     int block_size) {
    int bx = blockIdx.x;
    int tx = threadIdx.x;

    // Shared memory for the tile
    extern __shared__ int shared[];
    int *temp = shared;                        // (block_size+1) * (block_size+1)
    int *ref_shared = &shared[(block_size + 1) * (block_size + 1)];  // block_size

    // This kernel processes diagonal blocks in the upper-left triangle
    // block_idx is the diagonal index (0-based)
    // bx is which block along this diagonal

    if (bx > block_idx) return;

    int b_row = block_idx - bx;  // block row
    int b_col = bx;              // block col

    int begin_row = b_row * block_size + 1;
    int begin_col = b_col * block_size + 1;

    // Load reference values for this tile
    if (tx < block_size) {
        ref_shared[tx] = d_reference[begin_col + tx];
    }
    __syncthreads();

    // Load top and left borders into shared memory
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

    // Fill the tile using anti-diagonals
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

    // Write results back
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
    int bx = blockIdx.x;
    int tx = threadIdx.x;

    extern __shared__ int shared[];
    int *temp = shared;
    int *ref_shared = &shared[(block_size + 1) * (block_size + 1)];

    // Lower-right triangle diagonal
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

    // Ensure seq_len is a multiple of block_size
    int num_blocks = (seq_len + block_size - 1) / block_size;
    int padded_len = num_blocks * block_size;
    int cols = padded_len + 1;
    int rows = padded_len + 1;
    long matrix_size = (long)rows * cols;

    // Allocate host memory
    int *h_reference = (int *)malloc((padded_len + 1) * sizeof(int));
    int *h_matrix = (int *)malloc(matrix_size * sizeof(int));

    // Generate random sequences
    srand(42);
    for (int i = 0; i <= padded_len; i++) {
        h_reference[i] = rand() % 10;  // Simple alphabet of 10 chars
    }

    // Initialize DP matrix borders
    for (int i = 0; i < rows; i++) {
        h_matrix[i * cols] = -i * penalty;
    }
    for (int j = 0; j < cols; j++) {
        h_matrix[j] = -j * penalty;
    }

    // Device allocations
    int *d_reference, *d_matrix;
    CUDA_CHECK(cudaMalloc(&d_reference, (padded_len + 1) * sizeof(int)));
    CUDA_CHECK(cudaMalloc(&d_matrix, matrix_size * sizeof(int)));

    CUDA_CHECK(cudaMemcpy(d_reference, h_reference,
                          (padded_len + 1) * sizeof(int),
                          cudaMemcpyHostToDevice));

    int shared_mem_size = (block_size + 1) * (block_size + 1) * sizeof(int) +
                          block_size * sizeof(int);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", seq_len);

    printCSVHeader();

    BenchResult r = runBenchmark(
        "nw", problemSize, iterations, [&]() {
            // Reset DP matrix on device
            CUDA_CHECK(cudaMemcpy(d_matrix, h_matrix,
                                  matrix_size * sizeof(int),
                                  cudaMemcpyHostToDevice));

            // Phase 1: upper-left triangle diagonals
            for (int diag = 0; diag < num_blocks; diag++) {
                int blocks_on_diag = diag + 1;
                needle_cuda_shared_1<<<blocks_on_diag, block_size,
                                       shared_mem_size>>>(
                    d_reference, d_matrix, cols, penalty, diag, block_size);
            }

            // Phase 2: lower-right triangle diagonals
            for (int diag = 0; diag < num_blocks - 1; diag++) {
                int blocks_on_diag = num_blocks - 1 - diag;
                needle_cuda_shared_2<<<blocks_on_diag, block_size,
                                       shared_mem_size>>>(
                    d_reference, d_matrix, cols, penalty, diag, num_blocks,
                    block_size);
            }

            CUDA_CHECK(cudaDeviceSynchronize());
        });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_reference));
    CUDA_CHECK(cudaFree(d_matrix));
    free(h_reference);
    free(h_matrix);

    return 0;
}
