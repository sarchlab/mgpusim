// fdtd2d.cu: CUDA host program for PolyBench FDTD-2D.
//
// Kernels: amd/benchmarks/polybench/fdtd2d/native/polybench_fdtd2d.cpp
// Host:    follows amd/benchmarks/polybench/fdtd2d/fdtd2d.go
//          (three kernel launches per time step)
//
// Usage: fdtd2d [-size N] [-tmax T]
#include "check.h"
#include "polybench/fdtd2d/native/polybench_fdtd2d.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 16);
  const int tmax = intArg(argc, argv, "tmax", 10);
  std::printf("FDTD-2D: N=%d TMAX=%d\n", n, tmax);

  const size_t num = static_cast<size_t>(n) * n;
  std::vector<float> ex(num), ey(num), hz(num);
  for (size_t i = 0; i < num; i++) {
    ex[i] = static_cast<float>(i % 100) / 10.0f;
    ey[i] = static_cast<float>((i * 2) % 100) / 10.0f;
    hz[i] = static_cast<float>((i * 3) % 100) / 10.0f;
  }

  float* dEx = toDevice(ex);
  float* dEy = toDevice(ey);
  float* dHz = toDevice(hz);

  const dim3 grid(ceilDiv(n, BLOCK), ceilDiv(n, BLOCK));
  const dim3 block(BLOCK, BLOCK);
  for (int t = 0; t < tmax; t++) {
    fdtd_update_ex<<<grid, block>>>(dEx, dHz, n, n);
    CHECK_LAUNCH();
    fdtd_update_ey<<<grid, block>>>(dEy, dHz, n, n);
    CHECK_LAUNCH();
    fdtd_update_hz<<<grid, block>>>(dEx, dEy, dHz, n, n);
    CHECK_LAUNCH();
  }

  std::vector<float> gotEx = fromDevice(dEx, num);
  std::vector<float> gotEy = fromDevice(dEy, num);
  std::vector<float> gotHz = fromDevice(dHz, num);

  for (int t = 0; t < tmax; t++) {
    for (int i = 0; i < n; i++)
      for (int j = 0; j < n; j++)
        ex[i * n + j] = i == 0 ? 0.0f : ex[i * n + j] + 0.5f * (hz[i * n + j] - hz[(i - 1) * n + j]);
    for (int i = 0; i < n; i++)
      for (int j = 0; j < n; j++)
        ey[i * n + j] = j == 0 ? 0.0f : ey[i * n + j] + 0.5f * (hz[i * n + j] - hz[i * n + j - 1]);
    for (int i = 0; i < n - 1; i++)
      for (int j = 0; j < n - 1; j++)
        hz[i * n + j] -= 0.7f * (ex[i * n + j + 1] - ex[i * n + j] + ey[(i + 1) * n + j] - ey[i * n + j]);
  }

  int mismatches = countMismatches("ex", ex, gotEx) + countMismatches("ey", ey, gotEy) +
                   countMismatches("hz", hz, gotHz);
  cudaFree(dEx); cudaFree(dEy); cudaFree(dHz);
  return finish(mismatches);
}
