// jacobi2d.cu: CUDA host program for PolyBench Jacobi-2D.
//
// Kernel: amd/benchmarks/polybench/jacobi2d/native/polybench_jacobi2d.cpp
// Host:   follows amd/benchmarks/polybench/jacobi2d/jacobi2d.go
//         (one launch per time step, swapping the two grids)
//
// Usage: jacobi2d [-size N] [-tsteps T]
#include <utility>

#include "check.h"
#include "polybench/jacobi2d/native/polybench_jacobi2d.cpp"

static void cpuStep(const std::vector<float>& a, std::vector<float>& out, int n) {
  for (int i = 1; i < n - 1; i++)
    for (int j = 1; j < n - 1; j++)
      out[i * n + j] = (a[(i - 1) * n + j] + a[(i + 1) * n + j] + a[i * n + j - 1] +
                        a[i * n + j + 1] + a[i * n + j]) * 0.2f;
}

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 64);
  const int tsteps = intArg(argc, argv, "tsteps", 10);
  std::printf("JACOBI-2D: N=%d TSTEPS=%d\n", n, tsteps);

  std::vector<float> a(static_cast<size_t>(n) * n, 0.0f);
  for (int i = 1; i < n - 1; i++)
    for (int j = 1; j < n - 1; j++) a[i * n + j] = static_cast<float>((i * 7 + j * 13) % 100) / 10.0f;

  float* src = toDevice(a);
  float* dst = deviceAlloc<float>(a.size());

  const int g = ceilDiv(n - 2, BLOCK_DIM);
  for (int t = 0; t < tsteps; t++) {
    jacobi2d_kernel<<<dim3(g, g), dim3(BLOCK_DIM, BLOCK_DIM)>>>(src, dst, n);
    CHECK_LAUNCH();
    std::swap(src, dst);
  }

  std::vector<float> got = fromDevice(src, a.size());

  std::vector<float> ca = a, cb(a.size(), 0.0f);
  for (int t = 0; t < tsteps; t++) {
    cpuStep(ca, cb, n);
    std::swap(ca, cb);
  }

  int mismatches = countMismatches("A", ca, got);
  cudaFree(src); cudaFree(dst);
  return finish(mismatches);
}
