__kernel void gemm(int m, int n, int k, float alpha, float beta,
                   const __global float *a, const __global float *b,
                   const __global float *c, __global float *d) {
  const int TILE_SIZE = 16;
  __local float subTileM[TILE_SIZE][TILE_SIZE];
  __local float subTileN[TILE_SIZE][TILE_SIZE];

  int bx = get_global_id(0) / get_local_size(0);
  int by = get_global_id(1) / get_local_size(1);
  int tx = get_local_id(0);
  int ty = get_local_id(1);

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
      subTileM[ty][tx] = 0.0;
    }
    if (i * TILE_SIZE + ty < k && Col < n) {
      subTileN[ty][tx] = b[curR];
    } else {
      subTileN[ty][tx] = 0.0;
    }

    barrier(CLK_LOCAL_MEM_FENCE);
    for (int j = 0; j < TILE_SIZE; j++) {
      if (j + TILE_SIZE * i < m && j + TILE_SIZE * i < n) {
        Pvalue += subTileM[ty][j] *
                  subTileN[j][tx]; // subTileM[ty][j] * subTileN[j][tx];
      }
    }
    barrier(CLK_LOCAL_MEM_FENCE);
    // Pvalue = i * TILE_SIZE + ty;
  }
  if (Row < m && Col < n)
    d[Row * n + Col] = alpha * Pvalue + beta * c[Row * n + Col];
}

// __kernel void gemm(int m, int n, int k, float alpha, float beta,
//                    const __global float *a, const __global float *b,
//                    const __global float *c, __global float *d) {
//   int x = get_global_id(0);
//   int y = get_global_id(1);

//   if (y >= m || x >= n) {
//     return;
//   }

//   float acc = 0;
//   for (int z = 0; z < k; z++) {
//     acc += alpha * a[y * k + z] * b[z * n + x];
//   }

//   d[y * n + x] = acc + beta * c[y * n + x];
// }
