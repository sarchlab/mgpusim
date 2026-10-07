// conv2d.cu: CUDA host program for PolyBench 2D convolution.
//
// Kernel: amd/benchmarks/polybench/conv2d/native/polybench_2dconv.cpp
// Host:   follows amd/benchmarks/polybench/conv2d/conv2d.go
//
// Usage: conv2d [-size N]
#include "check.h"
#include "polybench/conv2d/native/polybench_2dconv.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 64);
  std::printf("2DCONV: N=%d\n", n);

  std::vector<float> a(static_cast<size_t>(n) * n);
  for (size_t i = 0; i < a.size(); i++) a[i] = static_cast<float>(i % 100) / 10.0f;

  float* dA = toDevice(a);
  float* dB = deviceAlloc<float>(a.size());

  dim3 grid(ceilDiv(n, BLOCK_SIZE), ceilDiv(n, BLOCK_SIZE));
  dim3 block(BLOCK_SIZE, BLOCK_SIZE);
  convolution2D_kernel<<<grid, block>>>(dA, dB, n, n);
  CHECK_LAUNCH();

  std::vector<float> b = fromDevice(dB, a.size());

  const float c[3][3] = {{0.8f, 0.2f, 0.3f}, {0.2f, 0.7f, 0.4f}, {0.1f, 0.2f, 0.5f}};
  std::vector<double> ref(a.size(), 0.0);
  for (int i = 1; i < n - 1; i++) {
    for (int j = 1; j < n - 1; j++) {
      double sum = 0.0;
      for (int di = -1; di <= 1; di++) {
        for (int dj = -1; dj <= 1; dj++) {
          sum += c[di + 1][dj + 1] * a[(i + di) * n + (j + dj)];
        }
      }
      ref[i * n + j] = sum;
    }
  }

  int mismatches = countMismatches("B", ref, b);
  cudaFree(dA); cudaFree(dB);
  return finish(mismatches);
}
