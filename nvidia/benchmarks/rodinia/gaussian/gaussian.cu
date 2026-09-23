// gaussian.cu: CUDA host program for Rodinia Gaussian elimination.
//
// Kernels: amd/benchmarks/rodinia/gaussian/native/rodinia_gaussian.cpp
// Host:    follows amd/benchmarks/rodinia/gaussian/gaussian.go
//          (fan1 + fan2 per pivot, then back substitution on the host)
//
// Usage: gaussian [-size N]
#include "check.h"
#include "rodinia/gaussian/native/rodinia_gaussian.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 64);
  std::printf("Gaussian: N=%d\n", n);

  std::vector<float> a(static_cast<size_t>(n) * n), b(n);
  for (size_t i = 0; i < a.size(); i++) a[i] = static_cast<float>((i * 7 + 3) % 9) / 10.0f + 0.1f;
  for (int i = 0; i < n; i++) a[i * n + i] += static_cast<float>(n);  // diagonally dominant
  for (int i = 0; i < n; i++) b[i] = static_cast<float>((i * 3 + 1) % 9) / 10.0f + 1.0f;

  float* dA = toDevice(a);
  float* dB = toDevice(b);
  float* dM = deviceAlloc<float>(a.size());

  for (int t = 0; t < n - 1; t++) {
    const int remaining = n - t - 1;
    fan1<<<ceilDiv(remaining, BLOCK1D), BLOCK1D>>>(dM, dA, n, t);
    CHECK_LAUNCH();
    const int g = ceilDiv(remaining, BLOCK2D);
    fan2<<<dim3(g, g), dim3(BLOCK2D, BLOCK2D)>>>(dM, dA, dB, n, t);
    CHECK_LAUNCH();
  }

  std::vector<float> ua = fromDevice(dA, a.size());
  std::vector<float> ub = fromDevice(dB, n);

  // Back substitution, then check the residual of the original system.
  std::vector<double> x(n);
  for (int i = n - 1; i >= 0; i--) {
    double sum = ub[i];
    for (int j = i + 1; j < n; j++) sum -= static_cast<double>(ua[i * n + j]) * x[j];
    x[i] = sum / ua[i * n + i];
  }
  double normRes = 0.0, normB = 0.0;
  for (int i = 0; i < n; i++) {
    double res = -static_cast<double>(b[i]);
    for (int j = 0; j < n; j++) res += static_cast<double>(a[i * n + j]) * x[j];
    normRes += res * res;
    normB += static_cast<double>(b[i]) * b[i];
  }
  const double relErr = std::sqrt(normRes / (normB + 1e-30));
  std::printf("relative residual = %.3e\n", relErr);

  cudaFree(dA); cudaFree(dB); cudaFree(dM);
  return finish(std::isfinite(relErr) && relErr < 1e-3 ? 0 : 1);
}
