// threemm.cu: CUDA host program for PolyBench 3MM (G = (A*B) * (C*D)).
//
// Kernels: amd/benchmarks/polybench/threemm/native/polybench_3mm.cpp
// Host:    follows amd/benchmarks/polybench/threemm/threemm.go
//          (the sample sets NI = NJ = NK = NL = NM = size)
//
// Usage: threemm [-size N]
#include "check.h"
#include "polybench/threemm/native/polybench_3mm.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 64);
  const int ni = n, nj = n, nk = n, nl = n, nm = n;
  std::printf("3MM: N=%d\n", n);

  std::vector<float> a(ni * nk), b(nk * nj), c(nj * nm), d(nm * nl);
  for (size_t i = 0; i < a.size(); i++) a[i] = static_cast<float>(i % 100) / 10.0f;
  for (size_t i = 0; i < b.size(); i++) b[i] = static_cast<float>((i * 2) % 100) / 10.0f;
  for (size_t i = 0; i < c.size(); i++) c[i] = static_cast<float>((i * 3) % 100) / 10.0f;
  for (size_t i = 0; i < d.size(); i++) d[i] = static_cast<float>((i * 4) % 100) / 10.0f;

  float* dA = toDevice(a);
  float* dB = toDevice(b);
  float* dC = toDevice(c);
  float* dD = toDevice(d);
  float* dE = deviceAlloc<float>(ni * nj);
  float* dF = deviceAlloc<float>(nj * nl);
  float* dG = deviceAlloc<float>(ni * nl);

  const dim3 block(BLOCK_SIZE, BLOCK_SIZE);
  mm3_kernel1<<<dim3(ceilDiv(nj, BLOCK_SIZE), ceilDiv(ni, BLOCK_SIZE)), block>>>(dA, dB, dE, ni, nk, nj);
  CHECK_LAUNCH();
  mm3_kernel2<<<dim3(ceilDiv(nl, BLOCK_SIZE), ceilDiv(nj, BLOCK_SIZE)), block>>>(dC, dD, dF, nj, nm, nl);
  CHECK_LAUNCH();
  mm3_kernel3<<<dim3(ceilDiv(nl, BLOCK_SIZE), ceilDiv(ni, BLOCK_SIZE)), block>>>(dE, dF, dG, ni, nj, nl);
  CHECK_LAUNCH();

  std::vector<float> got = fromDevice(dG, ni * nl);

  std::vector<double> e(ni * nj, 0.0), f(nj * nl, 0.0), g(ni * nl, 0.0);
  for (int i = 0; i < ni; i++)
    for (int j = 0; j < nj; j++)
      for (int k = 0; k < nk; k++) e[i * nj + j] += static_cast<double>(a[i * nk + k]) * b[k * nj + j];
  for (int j = 0; j < nj; j++)
    for (int l = 0; l < nl; l++)
      for (int m = 0; m < nm; m++) f[j * nl + l] += static_cast<double>(c[j * nm + m]) * d[m * nl + l];
  for (int i = 0; i < ni; i++)
    for (int l = 0; l < nl; l++)
      for (int j = 0; j < nj; j++) g[i * nl + l] += e[i * nj + j] * f[j * nl + l];

  int mismatches = countMismatches("G", g, got);
  cudaFree(dA); cudaFree(dB); cudaFree(dC); cudaFree(dD);
  cudaFree(dE); cudaFree(dF); cudaFree(dG);
  return finish(mismatches);
}
