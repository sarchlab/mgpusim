// mvt.cu: CUDA host program for PolyBench MVT (x1 = A*y1, x2 = A^T*y2).
//
// Kernels: amd/benchmarks/polybench/mvt/native/polybench_mvt.cpp
// Host:    follows amd/benchmarks/polybench/mvt/mvt.go
//
// Usage: mvt [-size N]
#include "check.h"
#include "polybench/mvt/native/polybench_mvt.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 128);
  std::printf("MVT: N=%d\n", n);

  std::vector<float> a(static_cast<size_t>(n) * n), y1(n), y2(n);
  for (size_t i = 0; i < a.size(); i++) a[i] = static_cast<float>(i % 100) / 10.0f;
  for (int j = 0; j < n; j++) {
    y1[j] = static_cast<float>((j * 3) % 100) / 10.0f;
    y2[j] = static_cast<float>((j * 7) % 100) / 10.0f;
  }

  float* dA = toDevice(a);
  float* dY1 = toDevice(y1);
  float* dY2 = toDevice(y2);
  float* dX1 = deviceAlloc<float>(n);
  float* dX2 = deviceAlloc<float>(n);

  mvt_kernel1<<<ceilDiv(n, BLOCK_SIZE), BLOCK_SIZE>>>(dA, dY1, dX1, n);
  CHECK_LAUNCH();
  mvt_kernel2<<<ceilDiv(n, BLOCK_SIZE), BLOCK_SIZE>>>(dA, dY2, dX2, n);
  CHECK_LAUNCH();

  std::vector<float> x1 = fromDevice(dX1, n);
  std::vector<float> x2 = fromDevice(dX2, n);

  std::vector<double> ref1(n, 0.0), ref2(n, 0.0);
  for (int i = 0; i < n; i++)
    for (int j = 0; j < n; j++) {
      ref1[i] += static_cast<double>(a[i * n + j]) * y1[j];
      ref2[j] += static_cast<double>(a[i * n + j]) * y2[i];
    }

  int mismatches = countMismatches("x1", ref1, x1) + countMismatches("x2", ref2, x2);
  cudaFree(dA); cudaFree(dY1); cudaFree(dY2); cudaFree(dX1); cudaFree(dX2);
  return finish(mismatches);
}
