// twomm.cu: CUDA host program for PolyBench 2MM
// (D = alpha*A*B + beta*D, then E = alpha*C*D + beta*E).
//
// Kernel: amd/benchmarks/polybench/twomm/native/polybench_2mm.cpp
// Host:   follows amd/benchmarks/polybench/twomm/twomm.go
//         (the same kernel is launched twice)
//
// Usage: twomm [-size N]
#include "check.h"
#include "polybench/twomm/native/polybench_2mm.cpp"

static const float kAlpha = 1.5f;
static const float kBeta = 1.2f;

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 128);
  std::printf("2MM: N=%d\n", n);

  const size_t num = static_cast<size_t>(n) * n;
  std::vector<float> a(num), b(num), c(num), d(num), e(num);
  for (size_t i = 0; i < num; i++) {
    a[i] = static_cast<float>(i % 100) / 100.0f;
    b[i] = static_cast<float>((i * 2) % 100) / 100.0f;
    c[i] = static_cast<float>((i * 3) % 100) / 100.0f;
    d[i] = static_cast<float>((i * 4) % 100) / 100.0f;
    e[i] = static_cast<float>((i * 5) % 100) / 100.0f;
  }

  float* dA = toDevice(a);
  float* dB = toDevice(b);
  float* dC = toDevice(c);
  float* dD = toDevice(d);
  float* dE = toDevice(e);

  const int g = ceilDiv(n, TILE_SIZE);
  const dim3 grid(g, g), block(TILE_SIZE, TILE_SIZE);
  mm_kernel<<<grid, block>>>(dA, dB, dD, n, kAlpha, kBeta);
  CHECK_LAUNCH();
  mm_kernel<<<grid, block>>>(dC, dD, dE, n, kAlpha, kBeta);
  CHECK_LAUNCH();

  std::vector<float> got = fromDevice(dE, num);

  std::vector<float> refD(num);
  for (int i = 0; i < n; i++)
    for (int j = 0; j < n; j++) {
      double sum = 0.0;
      for (int k = 0; k < n; k++) sum += static_cast<double>(a[i * n + k]) * b[k * n + j];
      refD[i * n + j] = static_cast<float>(kAlpha * sum + static_cast<double>(kBeta) * d[i * n + j]);
    }
  std::vector<double> refE(num);
  for (int i = 0; i < n; i++)
    for (int j = 0; j < n; j++) {
      double sum = 0.0;
      for (int k = 0; k < n; k++) sum += static_cast<double>(c[i * n + k]) * refD[k * n + j];
      refE[i * n + j] = kAlpha * sum + static_cast<double>(kBeta) * e[i * n + j];
    }

  // The AMD benchmark also allows 1e-2 here: the error of D carries into E.
  int mismatches = countMismatches("E", refE, got, 1e-2);
  cudaFree(dA); cudaFree(dB); cudaFree(dC); cudaFree(dD); cudaFree(dE);
  return finish(mismatches);
}
