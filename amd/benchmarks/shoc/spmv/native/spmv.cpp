#include "hip/hip_runtime.h"

#define SINGLE_PRECISION
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
