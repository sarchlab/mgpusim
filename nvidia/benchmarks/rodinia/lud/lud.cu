// lud.cu: CUDA host program for Rodinia LUD (blocked LU decomposition).
//
// Kernels: amd/benchmarks/rodinia/lud/native/rodinia_lud.cpp
// Host:    follows amd/benchmarks/rodinia/lud/lud.go
//          (per diagonal block: diagonal, perimeter, internal)
//
// Usage: lud [-size N]   (N must be a multiple of 16)
#include "check.h"
#include "rodinia/lud/native/rodinia_lud.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 128);
  if (n % BSIZE != 0) {
    std::fprintf(stderr, "size must be a multiple of %d\n", BSIZE);
    return 2;
  }
  std::printf("LUD: N=%d\n", n);

  // Diagonally dominant matrix from a 64-bit LCG, as in lud.go.
  std::vector<float> a(static_cast<size_t>(n) * n);
  unsigned long long state = 42;
  for (int i = 0; i < n; i++) {
    float rowSum = 0.0f;
    for (int j = 0; j < n; j++) {
      state = state * 6364136223846793005ull + 1442695040888963407ull;
      const float v = static_cast<float>(static_cast<int>((state >> 33) & 0x7fffffff) % 10 + 1) * 0.1f;
      a[i * n + j] = v;
      if (i != j) rowSum += std::fabs(v);
    }
    a[i * n + i] = rowSum + 1.0f;
  }

  float* dA = toDevice(a);

  const int numBlocks = n / BSIZE;
  const dim3 block(BSIZE, BSIZE);
  for (int k = 0; k < numBlocks; k++) {
    lud_diagonal<<<1, block>>>(dA, n, k);
    CHECK_LAUNCH();
    const int remaining = numBlocks - k - 1;
    if (remaining > 0) {
      lud_perimeter<<<2 * remaining, block>>>(dA, n, k);
      CHECK_LAUNCH();
      lud_internal<<<dim3(remaining, remaining), block>>>(dA, n, k);
      CHECK_LAUNCH();
    }
  }

  std::vector<float> lu = fromDevice(dA, a.size());

  // ||A - L*U||_F / ||A||_F, with L unit lower triangular.
  double err = 0.0, normA = 0.0;
  for (int i = 0; i < n; i++)
    for (int j = 0; j < n; j++) {
      double sum = 0.0;
      for (int k = 0; k <= std::min(i, j); k++)
        sum += (k == i ? 1.0 : static_cast<double>(lu[i * n + k])) * lu[k * n + j];
      const double diff = a[i * n + j] - sum;
      err += diff * diff;
      normA += static_cast<double>(a[i * n + j]) * a[i * n + j];
    }
  const double rel = std::sqrt(err / normA);
  std::printf("||A - LU||_F / ||A||_F = %.6e\n", rel);

  cudaFree(dA);
  return finish(rel < 1e-4 ? 0 : 1);
}
