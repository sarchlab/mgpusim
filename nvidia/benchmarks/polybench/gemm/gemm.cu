// gemm.cu: CUDA host program for PolyBench GEMM (C = alpha*A*B + beta*C).
//
// Kernel: amd/benchmarks/polybench/gemm/native/polybench_gemm.cpp
// Host:   follows amd/benchmarks/polybench/gemm/gemm.go
//
// Usage: gemm [-size N]
#include "check.h"
#include "polybench/gemm/native/polybench_gemm.cpp"

static const float kAlpha = 1.5f;
static const float kBeta = 1.2f;

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 512);
  std::printf("GEMM: N=%d\n", n);

  const size_t num = static_cast<size_t>(n) * n;
  std::vector<float> a(num), b(num), c(num);
  for (size_t i = 0; i < num; i++) {
    a[i] = static_cast<float>(i % 100) / 100.0f;
    b[i] = static_cast<float>((i * 2) % 100) / 100.0f;
    c[i] = static_cast<float>((i * 3) % 100) / 100.0f;
  }

  float* dA = toDevice(a);
  float* dB = toDevice(b);
  float* dC = toDevice(c);

  const int g = ceilDiv(n, TILE_SIZE);
  polybench_gemm_kernel<<<dim3(g, g), dim3(TILE_SIZE, TILE_SIZE)>>>(
      dA, dB, dC, n, kAlpha, kBeta);
  CHECK_LAUNCH();

  std::vector<float> got = fromDevice(dC, num);

  std::vector<double> ref(num);
  for (int i = 0; i < n; i++) {
    for (int j = 0; j < n; j++) {
      double sum = 0.0;
      for (int k = 0; k < n; k++) sum += static_cast<double>(a[i * n + k]) * b[k * n + j];
      ref[i * n + j] = kAlpha * sum + static_cast<double>(kBeta) * c[i * n + j];
    }
  }

  int mismatches = countMismatches("C", ref, got);
  cudaFree(dA); cudaFree(dB); cudaFree(dC);
  return finish(mismatches);
}
