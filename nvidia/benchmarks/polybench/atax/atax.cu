// atax.cu: CUDA host program for PolyBench ATAX (y = A^T * (A * x)).
//
// Kernels: amd/benchmarks/polybench/atax/native/atax.cpp
// Host:    follows amd/benchmarks/polybench/atax/benchmark.go
//
// Usage: atax [-x NX] [-y NY]
#include "check.h"
#include "polybench/atax/native/atax.cpp"

static const int kBlockSize = 256;

int main(int argc, char** argv) {
  const int nx = intArg(argc, argv, "x", 1024);
  const int ny = intArg(argc, argv, "y", 1024);
  std::printf("ATAX: NX=%d NY=%d\n", nx, ny);

  std::vector<float> a(static_cast<size_t>(nx) * ny), x(ny);
  for (int j = 0; j < ny; j++) x[j] = static_cast<float>(j) * static_cast<float>(M_PI);
  for (int i = 0; i < nx; i++)
    for (int j = 0; j < ny; j++)
      a[static_cast<size_t>(i) * ny + j] = static_cast<float>(i) * static_cast<float>(j) / nx;

  float* dA = toDevice(a);
  float* dX = toDevice(x);
  float* dY = deviceAlloc<float>(ny);
  float* dTmp = deviceAlloc<float>(nx);

  atax_kernel1<<<ceilDiv(nx, kBlockSize), kBlockSize>>>(dA, dX, dTmp, nx, ny);
  CHECK_LAUNCH();
  atax_kernel2<<<ceilDiv(ny, kBlockSize), kBlockSize>>>(dA, dY, dTmp, nx, ny);
  CHECK_LAUNCH();

  std::vector<float> y = fromDevice(dY, ny);

  std::vector<double> tmp(nx, 0.0), ref(ny, 0.0);
  for (int i = 0; i < nx; i++)
    for (int j = 0; j < ny; j++) tmp[i] += a[static_cast<size_t>(i) * ny + j] * x[j];
  for (int i = 0; i < nx; i++)
    for (int j = 0; j < ny; j++) ref[j] += a[static_cast<size_t>(i) * ny + j] * tmp[i];

  int mismatches = countMismatches("y", ref, y);
  cudaFree(dA); cudaFree(dX); cudaFree(dY); cudaFree(dTmp);
  return finish(mismatches);
}
