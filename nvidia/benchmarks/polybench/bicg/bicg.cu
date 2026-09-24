// bicg.cu: CUDA host program for PolyBench BICG (q = A * p, s = A^T * r).
//
// Kernels: amd/benchmarks/polybench/bicg/native/bicg.cpp
// Host:    follows amd/benchmarks/polybench/bicg/benchmark.go
//
// Usage: bicg [-x NX] [-y NY]
#include "check.h"
#include "polybench/bicg/native/bicg.cpp"

static const int kBlockSize = 256;

int main(int argc, char** argv) {
  const int nx = intArg(argc, argv, "x", 1024);
  const int ny = intArg(argc, argv, "y", 1024);
  std::printf("BICG: NX=%d NY=%d\n", nx, ny);

  std::vector<float> a(static_cast<size_t>(nx) * ny), r(nx), p(ny);
  for (int i = 0; i < nx; i++) {
    r[i] = static_cast<float>(i) * static_cast<float>(M_PI);
    for (int j = 0; j < ny; j++) {
      a[static_cast<size_t>(i) * ny + j] =
          static_cast<float>(i) * static_cast<float>(j) / nx;
    }
  }
  for (int j = 0; j < ny; j++) p[j] = static_cast<float>(j) * static_cast<float>(M_PI);

  float* dA = toDevice(a);
  float* dR = toDevice(r);
  float* dP = toDevice(p);
  float* dS = deviceAlloc<float>(ny);
  float* dQ = deviceAlloc<float>(nx);

  bicgKernel1<<<ceilDiv(nx, kBlockSize), kBlockSize>>>(dA, dP, dQ, nx, ny);
  CHECK_LAUNCH();
  bicgKernel2<<<ceilDiv(ny, kBlockSize), kBlockSize>>>(dA, dR, dS, nx, ny);
  CHECK_LAUNCH();

  std::vector<float> s = fromDevice(dS, ny);
  std::vector<float> q = fromDevice(dQ, nx);

  std::vector<double> refS(ny, 0.0), refQ(nx, 0.0);
  for (int i = 0; i < nx; i++) {
    for (int j = 0; j < ny; j++) {
      refS[j] += r[i] * a[static_cast<size_t>(i) * ny + j];
      refQ[i] += p[j] * a[static_cast<size_t>(i) * ny + j];
    }
  }

  int mismatches = countMismatches("s", refS, s) + countMismatches("q", refQ, q);
  cudaFree(dA); cudaFree(dR); cudaFree(dP); cudaFree(dS); cudaFree(dQ);
  return finish(mismatches);
}
