// spmv.cpp -- HIP benchmark for Parboil SpMV (Sparse Matrix-Vector Multiplication)
// CSR format: each thread computes one row of y = A*x

#include "bench_common_hip.h"

// SpMV CSR kernel: each thread computes one output element
__global__ void spmv_csr(const int *row_ptr, const int *col_idx,
                         const float *values, const float *x, float *y,
                         int num_rows) {
    int row = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (row < num_rows) {
        float sum = 0.0f;
        int row_start = row_ptr[row];
        int row_end = row_ptr[row + 1];
        for (int j = row_start; j < row_end; j++) {
            sum += values[j] * x[col_idx[j]];
        }
        y[row] = sum;
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_rows = parseIntParam(argc, argv, "--num_rows", 65536);
    int avg_nnz = parseIntParam(argc, argv, "--avg_nnz_per_row", 10);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int seed = parseIntParam(argc, argv, "--seed", 42);

    // Generate random sparse matrix in CSR format
    srand(seed);

    // First pass: determine number of non-zeros per row
    int *nnz_per_row = (int *)malloc(num_rows * sizeof(int));
    int total_nnz = 0;
    for (int i = 0; i < num_rows; i++) {
        // Random nnz per row: between 1 and 2*avg_nnz - 1 (uniform around avg)
        int nnz = 1 + (rand() % (2 * avg_nnz - 1));
        if (nnz > num_rows) nnz = num_rows;
        nnz_per_row[i] = nnz;
        total_nnz += nnz;
    }

    // Build CSR arrays
    int *h_row_ptr = (int *)malloc((num_rows + 1) * sizeof(int));
    int *h_col_idx = (int *)malloc(total_nnz * sizeof(int));
    float *h_values = (float *)malloc(total_nnz * sizeof(float));
    float *h_x = (float *)malloc(num_rows * sizeof(float));
    float *h_y = (float *)malloc(num_rows * sizeof(float));

    // Build row_ptr
    h_row_ptr[0] = 0;
    for (int i = 0; i < num_rows; i++) {
        h_row_ptr[i + 1] = h_row_ptr[i] + nnz_per_row[i];
    }

    // Fill column indices and values
    for (int i = 0; i < num_rows; i++) {
        int start = h_row_ptr[i];
        int nnz = nnz_per_row[i];
        // Generate sorted unique column indices
        for (int j = 0; j < nnz; j++) {
            h_col_idx[start + j] = (int)(((long long)j * num_rows) / nnz + rand() % ((num_rows / nnz) > 0 ? (num_rows / nnz) : 1)) % num_rows;
            h_values[start + j] = (float)(rand() % 100) / 100.0f;
        }
        // Sort column indices for this row (simple insertion sort)
        for (int j = 1; j < nnz; j++) {
            int key_col = h_col_idx[start + j];
            float key_val = h_values[start + j];
            int k = j - 1;
            while (k >= 0 && h_col_idx[start + k] > key_col) {
                h_col_idx[start + k + 1] = h_col_idx[start + k];
                h_values[start + k + 1] = h_values[start + k];
                k--;
            }
            h_col_idx[start + k + 1] = key_col;
            h_values[start + k + 1] = key_val;
        }
    }

    // Initialize x vector
    for (int i = 0; i < num_rows; i++) {
        h_x[i] = (float)(rand() % 100) / 100.0f;
    }

    free(nnz_per_row);

    // Device allocations
    int *d_row_ptr, *d_col_idx;
    float *d_values, *d_x, *d_y;
    HIP_CHECK(hipMalloc(&d_row_ptr, (num_rows + 1) * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_col_idx, total_nnz * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_values, total_nnz * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_x, num_rows * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_y, num_rows * sizeof(float)));

    HIP_CHECK(hipMemcpy(d_row_ptr, h_row_ptr, (num_rows + 1) * sizeof(int),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_col_idx, h_col_idx, total_nnz * sizeof(int),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_values, h_values, total_nnz * sizeof(float),
                        hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_x, h_x, num_rows * sizeof(float),
                        hipMemcpyHostToDevice));

    // Kernel launch config
    dim3 block(block_size);
    dim3 grid((num_rows + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_rows);

    printCSVHeader();

    BenchResult r = runBenchmark(
        "spmv_csr", problemSize, iterations, [&]() {
            spmv_csr<<<grid, block>>>(d_row_ptr, d_col_idx, d_values, d_x, d_y,
                                      num_rows);
        });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_row_ptr));
    HIP_CHECK(hipFree(d_col_idx));
    HIP_CHECK(hipFree(d_values));
    HIP_CHECK(hipFree(d_x));
    HIP_CHECK(hipFree(d_y));
    free(h_row_ptr);
    free(h_col_idx);
    free(h_values);
    free(h_x);
    free(h_y);

    return 0;
}
