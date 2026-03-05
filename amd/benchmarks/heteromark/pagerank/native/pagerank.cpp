/**
 * pagerank.cpp: HIP implementation of PageRank kernel
 * Translated from kernels.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

extern "C" __global__ void PageRankUpdateGpu(unsigned int num_rows,
                                             unsigned int *rowOffset,
                                             unsigned int *col, float *val,
                                             float *x, float *y) {
  // Shared memory for warp reduction - 64 floats per workgroup
  __shared__ float vals[64];

  int thread_id = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  int local_id = hipThreadIdx_x;
  int warp_id = thread_id / 64;
  int lane = thread_id & (64 - 1);
  int row = warp_id;

  if (row < num_rows) {
    y[row] = 0.0;
    int row_A_start = rowOffset[row];
    int row_A_end = rowOffset[row + 1];

    vals[local_id] = 0;
    for (int jj = row_A_start + lane; jj < row_A_end; jj += 64)
      vals[local_id] += val[jj] * x[col[jj]];

    // Warp reduction: 64 -> 32 -> 16 -> 8 -> 4 -> 2 -> 1
    if (lane < 32) vals[local_id] += vals[local_id + 32];
    if (lane < 16) vals[local_id] += vals[local_id + 16];
    if (lane < 8) vals[local_id] += vals[local_id + 8];
    if (lane < 4) vals[local_id] += vals[local_id + 4];
    if (lane < 2) vals[local_id] += vals[local_id + 2];
    if (lane < 1) vals[local_id] += vals[local_id + 1];
    if (lane == 0) y[row] += vals[local_id];
  }
}
