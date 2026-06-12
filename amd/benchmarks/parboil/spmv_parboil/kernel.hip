#include <hip/hip_runtime.h>

// SpMV CSR kernel: each thread computes one output element
// y = A * x, where A is stored in CSR (Compressed Sparse Row) format
extern "C" __global__ void spmv_csr(const int *row_ptr, const int *col_idx,
                                     const float *values, const float *x,
                                     float *y, int num_rows) {
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
