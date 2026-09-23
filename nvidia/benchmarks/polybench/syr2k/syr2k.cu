// syr2k.cu: CUDA host program for PolyBench SYR2K
// (C = alpha*(A*B^T + B*A^T) + beta*C).
//
// Kernel: amd/benchmarks/polybench/syr2k/native/polybench_syr2k.cpp
// Host:   follows amd/benchmarks/polybench/syr2k/syr2k.go
//
// Usage: syr2k [-size N] [-inner-size M]
#include "check.h"
#include "polybench/syr2k/native/polybench_syr2k.cpp"

static const float kAlpha = 1.5f;
static const float kBeta = 1.2f;

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 64);
  const int m = intArg(argc, argv, "inner-size", 64);
  std::printf("SYR2K: N=%d M=%d\n", n, m);

  std::vector<float> a(static_cast<size_t>(n) * m), b(a.size()), c(static_cast<size_t>(n) * n);
  for (size_t i = 0; i < a.size(); i++) {
    a[i] = static_cast<float>(i % 100) / 100.0f;
    b[i] = static_cast<float>((i * 2) % 100) / 100.0f;
  }
  for (size_t i = 0; i < c.size(); i++) c[i] = static_cast<float>((i * 3) % 100) / 100.0f;

  float* dA = toDevice(a);
  float* dB = toDevice(b);
  float* dC = toDevice(c);

  const int g = ceilDiv(n, TILE_SIZE);
  polybench_syr2k_kernel<<<dim3(g, g), dim3(TILE_SIZE, TILE_SIZE)>>>(dA, dB, dC, n, m, kAlpha, kBeta);
  CHECK_LAUNCH();

  std::vector<float> got = fromDevice(dC, c.size());

  std::vector<double> ref(c.size());
  for (int i = 0; i < n; i++)
    for (int j = 0; j < n; j++) {
      double sum = 0.0;
      for (int k = 0; k < m; k++)
        sum += static_cast<double>(a[i * m + k]) * b[j * m + k] +
               static_cast<double>(b[i * m + k]) * a[j * m + k];
      ref[i * n + j] = kAlpha * sum + static_cast<double>(kBeta) * c[i * n + j];
    }

  int mismatches = countMismatches("C", ref, got);
  cudaFree(dA); cudaFree(dB); cudaFree(dC);
  return finish(mismatches);
}
