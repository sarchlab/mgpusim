__kernel void gemm(int m, int n, int k, float alpha, float beta,
                   const __global float *a, const __global float *b,
                   const __global float *c, __global float *d) {
                   //TILE SIZE 8
    const int TILE_SIZE = 8;
    __local float subTileM[TILE_SIZE][TILE_SIZE];
    __local float subTileN[TILE_SIZE][TILE_SIZE];

    int bx = get_global_id(0)/get_local_id(0);
    int by = get_global_id(1)/get_local_id(1);
    int tx = get_local_id(0);
    int ty = get_local_id(1);

    int Row = by * TILE_SIZE + ty;
    int Col = bx * TILE_SIZE + tx;

    float Pvalue = 0;
    for(int i = 0; i <= k/TILE_SIZE; i++){
        int curL = Row * k + i * TILE_SIZE + tx;
        int curR = (i * TILE_SIZE +ty)*k + Col;
        if(curL < m * k && curR < n * k ){
            subTileM[ty][tx] = a[curL];
            subTileN[ty][tx] = b[curR];
            barrier();
            for (int j = 0; j < TILE_SIZE; ++j){
                Pvalue += alpha * subTileM[ty][k] * subTileN[k][tx];
                barrier();
            }
        }

    }
    d[Row * k + Col] = Pvalue + beta * c[Row * k + Col];
}



/*__kernel void gemm(int m, int n, int k, float alpha, float beta,
                   const __global float *a, const __global float *b,
                   const __global float *c, __global float *d) {
  int x = get_global_id(0);
  int y = get_global_id(1);

  if (y >= m || x >= n) {
    return;
  }

  float acc = 0;
  for (int z = 0; z < k; z++) {
    acc += alpha * a[y * k + z] * b[z * n + x];
  }

  d[y * n + x] = acc + beta * c[y * n + x];
}
*/

