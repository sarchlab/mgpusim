// gramschmidt.cu: CUDA host program for PolyBench Gram-Schmidt (A = QR).
//
// Kernels: amd/benchmarks/polybench/gramschmidt/native/polybench_gramschmidt.cpp
// Host:    follows amd/benchmarks/polybench/gramschmidt/gramschmidt.go
//          (per column k: norm, normalize, then project the remaining columns)
//
// Usage: gramschmidt [-m M] [-n N]
#include "check.h"
#include "polybench/gramschmidt/native/polybench_gramschmidt.cpp"

int main(int argc, char** argv) {
  const int m = intArg(argc, argv, "m", 32);
  const int n = intArg(argc, argv, "n", 32);
  std::printf("GRAMSCHMIDT: M=%d N=%d\n", m, n);

  std::vector<float> a(static_cast<size_t>(m) * n);
  unsigned state = 42;
  for (float& v : a) {
    state = state * 1103515245u + 12345u;
    v = static_cast<float>((state >> 16) & 0x7fff) / 32768.0f;
  }

  float* dA = toDevice(a);
  float* dQ = deviceAlloc<float>(a.size());
  float* dR = deviceAlloc<float>(static_cast<size_t>(n) * n);
  float* dNrm = deviceAlloc<float>(1);

  for (int k = 0; k < n; k++) {
    gram_norm_finish<<<1, BLOCK_SIZE>>>(dA, dR, dNrm, m, n, k);
    CHECK_LAUNCH();
    gram_normalize<<<ceilDiv(m, BLOCK_SIZE), BLOCK_SIZE>>>(dA, dQ, dNrm, m, n, k);
    CHECK_LAUNCH();
    if (k + 1 < n) {
      gram_project<<<ceilDiv(n - k - 1, BLOCK_SIZE), BLOCK_SIZE>>>(dA, dQ, dR, m, n, k);
      CHECK_LAUNCH();
    }
  }

  std::vector<float> q = fromDevice(dQ, a.size());
  std::vector<float> r = fromDevice(dR, static_cast<size_t>(n) * n);

  // CPU reference.
  std::vector<float> refQ(a.size(), 0.0f), refR(static_cast<size_t>(n) * n, 0.0f);
  for (int k = 0; k < n; k++) {
    float sum = 0.0f;
    for (int i = 0; i < m; i++) sum += a[i * n + k] * a[i * n + k];
    const float nrm = std::sqrt(sum);
    refR[k * n + k] = nrm;
    for (int i = 0; i < m; i++) refQ[i * n + k] = a[i * n + k] / nrm;
    for (int j = k + 1; j < n; j++) {
      float dot = 0.0f;
      for (int i = 0; i < m; i++) dot += refQ[i * n + k] * a[i * n + j];
      refR[k * n + j] = dot;
      for (int i = 0; i < m; i++) a[i * n + j] -= dot * refQ[i * n + k];
    }
  }

  int mismatches = countMismatches("Q", refQ, q) + countMismatches("R", refR, r);
  cudaFree(dA); cudaFree(dQ); cudaFree(dR); cudaFree(dNrm);
  return finish(mismatches);
}
