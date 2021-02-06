__kernel void gemm(int m, int n, int k, float alpha, float beta,
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
