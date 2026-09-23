/**
 * gemm_hip.cpp: HIP implementation of GEMM kernels
 * Translated from gemm.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

#define TILE_SIZE 16

extern "C" __global__ void gemm(int m, int n, int k, float alpha, float beta,
                   const float *a, const float *b,
                   const float *c, float *d) {
  __shared__ float subTileM[TILE_SIZE][TILE_SIZE];
  __shared__ float subTileN[TILE_SIZE][TILE_SIZE];

  int globalX = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  int globalY = hipBlockDim_y * hipBlockIdx_y + hipThreadIdx_y;
  int bx = globalX / hipBlockDim_x;
  int by = globalY / hipBlockDim_y;
  int tx = hipThreadIdx_x;
  int ty = hipThreadIdx_y;

  int Row = by * TILE_SIZE + ty;
  int Col = bx * TILE_SIZE + tx;

  d[Row * n + Col] = 0;
  float Pvalue = 0;
  for (int i = 0; i < ((k - 1) / TILE_SIZE + 1); i++) {
    int curL = Row * k + i * TILE_SIZE + tx;
    int curR = (i * TILE_SIZE + ty) * n + Col;

    if (i * TILE_SIZE + tx < k && Row < m) {
      subTileM[ty][tx] = a[curL];
    } else {
      subTileM[ty][tx] = 0.0f;
    }

    if (i * TILE_SIZE + ty < k && Col < n) {
      subTileN[ty][tx] = b[curR];
    } else {
      subTileN[ty][tx] = 0.0f;
    }

    __syncthreads();
    for (int j = 0; j < TILE_SIZE; j++) {
      if (j + TILE_SIZE * i < k) {
        Pvalue += subTileM[ty][j] * subTileN[j][tx];
      }
    }
    __syncthreads();
  }

  if (Row < m && Col < n) {
    d[Row * n + Col] = alpha * Pvalue + beta * c[Row * n + Col];
  }
}

extern "C" __global__ void gemm_old(int m, int n, int k, float alpha, float beta,
                       const float *a, const float *b,
                       const float *c, float *d) {
  int x = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  int y = hipBlockDim_y * hipBlockIdx_y + hipThreadIdx_y;

  if (y >= m || x >= n) {
    return;
  }

  float acc = 0;
  for (int z = 0; z < k; z++) {
    acc += alpha * a[y * k + z] * b[z * n + x];
  }

  d[y * n + x] = acc + beta * c[y * n + x];
}
