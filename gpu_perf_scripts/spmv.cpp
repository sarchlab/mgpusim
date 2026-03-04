// spmv.cpp — HIP benchmark for sparse matrix-vector multiplication (CSR)
// Kernel copied from amd/benchmarks/shoc/spmv/native/spmv.cpp
// Problem size: 1024 rows, ~10 nnz/row

#include "bench_common.h"

#define FPTYPE float

extern "C" __global__ void spmv_csr_scalar_kernel(
    const FPTYPE * __restrict__ val,
    const FPTYPE * __restrict__ vec,
    const int * __restrict__ cols,
    const int * __restrict__ rowDelimiters,
    const int dim,
    FPTYPE * __restrict__ out)
{
    int myRow = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    if (myRow < dim) {
        FPTYPE t = 0;
        int start = rowDelimiters[myRow];
        int end = rowDelimiters[myRow + 1];
        for (int j = start; j < end; j++) {
            int col = cols[j];
            t += val[j] * vec[col];
        }
        out[myRow] = t;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    int NUM_ROWS = parseIntParam(argc, argv, "--rows", 1024);
    const int NNZ_PER_ROW = 10;
    const int TOTAL_NNZ = NUM_ROWS * NNZ_PER_ROW;

    // Host: build CSR sparse matrix
    std::vector<int> h_rowDelimiters(NUM_ROWS + 1);
    std::vector<int> h_cols(TOTAL_NNZ);
    std::vector<float> h_val(TOTAL_NNZ);
    std::vector<float> h_vec(NUM_ROWS);
    std::vector<float> h_out(NUM_ROWS, 0.0f);

    // Fill row delimiters (uniform nnz per row)
    for (int i = 0; i <= NUM_ROWS; i++) {
        h_rowDelimiters[i] = i * NNZ_PER_ROW;
    }

    // Fill column indices and values
    srand(42);
    for (int r = 0; r < NUM_ROWS; r++) {
        for (int j = 0; j < NNZ_PER_ROW; j++) {
            h_cols[r * NNZ_PER_ROW + j] = (r + j) % NUM_ROWS;
            h_val[r * NNZ_PER_ROW + j] = (float)(rand() % 100) / 50.0f;
        }
    }

    // Fill vector
    for (int i = 0; i < NUM_ROWS; i++) {
        h_vec[i] = (float)(rand() % 100) / 100.0f;
    }

    // Device allocations
    float *d_val, *d_vec, *d_out;
    int *d_cols, *d_rowDelimiters;

    HIP_CHECK(hipMalloc(&d_val, TOTAL_NNZ * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_vec, NUM_ROWS * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_out, NUM_ROWS * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_cols, TOTAL_NNZ * sizeof(int)));
    HIP_CHECK(hipMalloc(&d_rowDelimiters, (NUM_ROWS + 1) * sizeof(int)));

    HIP_CHECK(hipMemcpy(d_val, h_val.data(), TOTAL_NNZ * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_vec, h_vec.data(), NUM_ROWS * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_cols, h_cols.data(), TOTAL_NNZ * sizeof(int), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_rowDelimiters, h_rowDelimiters.data(), (NUM_ROWS + 1) * sizeof(int), hipMemcpyHostToDevice));

    const int THREADS = 256;
    dim3 block(THREADS);
    dim3 grid((NUM_ROWS + THREADS - 1) / THREADS);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d_nnz%d", NUM_ROWS, NUM_ROWS, TOTAL_NNZ);

    BenchResult r = runBenchmark("spmv_csr_scalar", problemSize, iterations, [&]() {
        spmv_csr_scalar_kernel<<<grid, block>>>(d_val, d_vec, d_cols, d_rowDelimiters, NUM_ROWS, d_out);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_val));
    HIP_CHECK(hipFree(d_vec));
    HIP_CHECK(hipFree(d_out));
    HIP_CHECK(hipFree(d_cols));
    HIP_CHECK(hipFree(d_rowDelimiters));

    return 0;
}
