// atax.cu: CUDA host program for PolyBench ATAX (y = A^T * (A * x)).
//
// The kernels are not copied here. They are compiled from the same source as
// the AMD benchmark, amd/benchmarks/polybench/atax/native/atax.cpp, and this
// host program follows amd/benchmarks/polybench/atax/benchmark.go.
//
// Usage: atax [-x NX] [-y NY]
#include <cmath>
#include <cstdio>
#include <vector>

#include "check.h"
#include "polybench/atax/native/atax.cpp"

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

static const int kBlockSize = 256;

int main(int argc, char** argv) {
  const int nx = intArg(argc, argv, "x", 1024);
  const int ny = intArg(argc, argv, "y", 1024);
  std::printf("ATAX: NX=%d NY=%d\n", nx, ny);

  std::vector<float> a(static_cast<size_t>(nx) * ny);
  std::vector<float> x(ny);
  std::vector<float> y(ny);

  for (int j = 0; j < ny; j++) {
    x[j] = static_cast<float>(j) * static_cast<float>(M_PI);
  }
  for (int i = 0; i < nx; i++) {
    for (int j = 0; j < ny; j++) {
      a[static_cast<size_t>(i) * ny + j] =
          static_cast<float>(i) * static_cast<float>(j) / nx;
    }
  }

  float *dA, *dX, *dY, *dTmp;
  CHECK_CUDA(cudaMalloc(&dA, a.size() * sizeof(float)));
  CHECK_CUDA(cudaMalloc(&dX, x.size() * sizeof(float)));
  CHECK_CUDA(cudaMalloc(&dY, y.size() * sizeof(float)));
  CHECK_CUDA(cudaMalloc(&dTmp, nx * sizeof(float)));
  CHECK_CUDA(cudaMemset(dY, 0, y.size() * sizeof(float)));
  CHECK_CUDA(cudaMemset(dTmp, 0, nx * sizeof(float)));

  CHECK_CUDA(cudaMemcpy(dA, a.data(), a.size() * sizeof(float),
                        cudaMemcpyHostToDevice));
  CHECK_CUDA(cudaMemcpy(dX, x.data(), x.size() * sizeof(float),
                        cudaMemcpyHostToDevice));

  atax_kernel1<<<(nx + kBlockSize - 1) / kBlockSize, kBlockSize>>>(
      dA, dX, dTmp, nx, ny);
  CHECK_CUDA(cudaGetLastError());
  atax_kernel2<<<(ny + kBlockSize - 1) / kBlockSize, kBlockSize>>>(
      dA, dY, dTmp, nx, ny);
  CHECK_CUDA(cudaGetLastError());

  CHECK_CUDA(cudaMemcpy(y.data(), dY, y.size() * sizeof(float),
                        cudaMemcpyDeviceToHost));

  // Check against a CPU reference with the same summation order.
  std::vector<double> tmp(nx, 0.0);
  int mismatches = 0;
  for (int i = 0; i < nx; i++) {
    for (int j = 0; j < ny; j++) {
      tmp[i] += a[static_cast<size_t>(i) * ny + j] * x[j];
    }
  }
  for (int j = 0; j < ny; j++) {
    double ref = 0.0;
    for (int i = 0; i < nx; i++) {
      ref += a[static_cast<size_t>(i) * ny + j] * tmp[i];
    }
    if (std::fabs(ref - y[j]) > 1e-3 * std::fabs(ref) + 1e-3) {
      mismatches++;
    }
  }

  CHECK_CUDA(cudaFree(dA));
  CHECK_CUDA(cudaFree(dX));
  CHECK_CUDA(cudaFree(dY));
  CHECK_CUDA(cudaFree(dTmp));

  if (mismatches > 0) {
    std::printf("Failed: %d mismatches\n", mismatches);
    return 1;
  }
  std::printf("Passed!\n");
  return 0;
}
