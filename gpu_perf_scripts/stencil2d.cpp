// stencil2d.cpp — HIP benchmark for 2D stencil (9-point)
// Kernels copied from amd/benchmarks/shoc/stencil2d/native/stencil2d.cpp
// Problem size: 512x512 grid

#include "bench_common.h"

#define VALTYPE float
#define LROWS 16

__device__ inline int ToGlobalRow(int gidRow, int lszRow, int lidRow) {
    return gidRow * lszRow + lidRow;
}

__device__ inline int ToGlobalCol(int gidCol, int lszCol, int lidCol) {
    return gidCol * lszCol + lidCol;
}

__device__ inline int ToFlatHaloedIdx(int row, int col, int rowPitch) {
    return (row + 1) * (rowPitch + 2) + (col + 1);
}

__device__ inline int ToFlatIdx(int row, int col, int pitch) {
    return row * pitch + col;
}

__global__ void CopyRect(VALTYPE* dest, int doffset, int dpitch,
                          VALTYPE* src, int soffset, int spitch,
                          int width, int height) {
    int gid = hipBlockIdx_x;
    int lid = hipThreadIdx_x;
    int lsz = hipBlockDim_x;
    int grow = gid * lsz + lid;

    if (grow < height) {
        for (int c = 0; c < width; c++) {
            (dest + doffset)[ToFlatIdx(grow, c, dpitch)] =
                (src + soffset)[ToFlatIdx(grow, c, spitch)];
        }
    }
}

__global__ void StencilKernel(VALTYPE* data, VALTYPE* newData,
                               const int alignment, VALTYPE wCenter,
                               VALTYPE wCardinal, VALTYPE wDiagonal) {
    __shared__ VALTYPE sh[(LROWS + 2) * (64 + 2)];

    int gidRow = hipBlockIdx_x;
    int gidCol = hipBlockIdx_y;
    int gszRow = hipGridDim_x;
    int gszCol = hipGridDim_y;
    int lidRow = hipThreadIdx_x;
    int lidCol = hipThreadIdx_y;
    int lszRow = LROWS;
    int lszCol = hipBlockDim_y;

    int gRow = ToGlobalRow(gidRow, lszRow, lidRow);
    int gCol = ToGlobalCol(gidCol, lszCol, lidCol);

    int nCols = gszCol * lszCol + 2;
    int nPaddedCols =
        nCols +
        (((nCols % alignment) == 0) ? 0 : (alignment - (nCols % alignment)));
    int gRowWidth = nPaddedCols - 2;

    int lRowWidth = lszCol;
    for (int i = 0; i < (lszRow + 2); i++) {
        int lidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol, lRowWidth);
        int gidx = ToFlatHaloedIdx(gRow - 1 + i, gCol, gRowWidth);
        sh[lidx] = data[gidx];
    }

    if (lidCol == 0) {
        for (int i = 0; i < (lszRow + 2); i++) {
            int lidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol - 1, lRowWidth);
            int gidx = ToFlatHaloedIdx(gRow - 1 + i, gCol - 1, gRowWidth);
            sh[lidx] = data[gidx];
        }
    } else if (lidCol == (lszCol - 1)) {
        for (int i = 0; i < (lszRow + 2); i++) {
            int lidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol + 1, lRowWidth);
            int gidx = ToFlatHaloedIdx(gRow - 1 + i, gCol + 1, gRowWidth);
            sh[lidx] = data[gidx];
        }
    }

    __syncthreads();

    for (int i = 0; i < lszRow; i++) {
        int cidx = ToFlatHaloedIdx(lidRow + i, lidCol, lRowWidth);
        int nidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol, lRowWidth);
        int sidx = ToFlatHaloedIdx(lidRow + 1 + i, lidCol, lRowWidth);
        int eidx = ToFlatHaloedIdx(lidRow + i, lidCol + 1, lRowWidth);
        int widx = ToFlatHaloedIdx(lidRow + i, lidCol - 1, lRowWidth);
        int neidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol + 1, lRowWidth);
        int seidx = ToFlatHaloedIdx(lidRow + 1 + i, lidCol + 1, lRowWidth);
        int nwidx = ToFlatHaloedIdx(lidRow - 1 + i, lidCol - 1, lRowWidth);
        int swidx = ToFlatHaloedIdx(lidRow + 1 + i, lidCol - 1, lRowWidth);

        VALTYPE centerValue = sh[cidx];
        VALTYPE cardinalValueSum = sh[nidx] + sh[sidx] + sh[eidx] + sh[widx];
        VALTYPE diagonalValueSum = sh[neidx] + sh[seidx] + sh[nwidx] + sh[swidx];

        newData[ToFlatHaloedIdx(gRow + i, gCol, gRowWidth)] =
            wCenter * centerValue + wCardinal * cardinalValueSum +
            wDiagonal * diagonalValueSum;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);

    // Grid dimensions (logical, without halo)
    const int ROWS = 512;
    const int COLS = 512;
    const int ALIGNMENT = 16;

    // Total dimensions including halo (1-wide border)
    int totalCols = COLS + 2;
    int paddedCols = totalCols +
        (((totalCols % ALIGNMENT) == 0) ? 0 : (ALIGNMENT - (totalCols % ALIGNMENT)));
    int totalRows = ROWS + 2;
    int totalSize = totalRows * paddedCols;

    // Stencil weights
    VALTYPE wCenter   = 0.25f;
    VALTYPE wCardinal = 0.15f;
    VALTYPE wDiagonal = 0.05f;

    // Host allocation
    VALTYPE* h_data = (VALTYPE*)malloc(totalSize * sizeof(VALTYPE));
    srand(42);
    for (int i = 0; i < totalSize; i++)
        h_data[i] = (VALTYPE)(rand() % 100) / 10.0f;

    // Device allocations
    VALTYPE *d_data, *d_newData;
    HIP_CHECK(hipMalloc(&d_data, totalSize * sizeof(VALTYPE)));
    HIP_CHECK(hipMalloc(&d_newData, totalSize * sizeof(VALTYPE)));

    HIP_CHECK(hipMemcpy(d_data, h_data, totalSize * sizeof(VALTYPE), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_newData, h_data, totalSize * sizeof(VALTYPE), hipMemcpyHostToDevice));

    // Kernel launch config:
    // blockDim = (1, 64) => lidRow=0 always, lszCol=64
    // gridDim = (ROWS/LROWS, COLS/64) for the stencil kernel
    // Each block handles LROWS rows and 64 columns
    const int LOCAL_COLS = 64;
    dim3 block(1, LOCAL_COLS);
    dim3 grid(ROWS / LROWS, COLS / LOCAL_COLS);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", ROWS, COLS);

    BenchResult r = runBenchmark("stencil2d", problemSize, iterations, [&]() {
        StencilKernel<<<grid, block>>>(d_data, d_newData, ALIGNMENT,
                                       wCenter, wCardinal, wDiagonal);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_data));
    HIP_CHECK(hipFree(d_newData));
    free(h_data);

    return 0;
}
