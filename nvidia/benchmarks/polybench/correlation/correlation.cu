// correlation.cu: CUDA host program for PolyBench Correlation.
//
// Kernels: amd/benchmarks/polybench/correlation/native/polybench_correlation.cpp
// Host:    follows amd/benchmarks/polybench/correlation/correlation.go
//          (the data matrix is N x N, i.e., M == N)
//
// Usage: correlation [-size N]
#include "check.h"
#include "polybench/correlation/native/polybench_correlation.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 64);
  const int m = n;
  std::printf("CORRELATION: N=%d\n", n);

  std::vector<float> data(static_cast<size_t>(m) * n);
  for (size_t i = 0; i < data.size(); i++) {
    data[i] = static_cast<float>((i * 7 + 3) % 1000) / 100.0f;
  }

  float* dData = toDevice(data);
  float* dMean = deviceAlloc<float>(n);
  float* dStddev = deviceAlloc<float>(n);
  float* dCorr = deviceAlloc<float>(static_cast<size_t>(n) * n);

  mean_kernel<<<ceilDiv(n, BLOCK_SIZE), BLOCK_SIZE>>>(dData, dMean, m, n);
  CHECK_LAUNCH();
  stddev_kernel<<<ceilDiv(n, BLOCK_SIZE), BLOCK_SIZE>>>(dData, dMean, dStddev, m, n);
  CHECK_LAUNCH();
  normalize_kernel<<<ceilDiv(m * n, BLOCK_SIZE), BLOCK_SIZE>>>(dData, dMean, dStddev, m, n);
  CHECK_LAUNCH();
  const int g = ceilDiv(n, TILE_SIZE);
  correlation_kernel<<<dim3(g, g), dim3(TILE_SIZE, TILE_SIZE)>>>(dData, dCorr, m, n);
  CHECK_LAUNCH();

  std::vector<float> corr = fromDevice(dCorr, static_cast<size_t>(n) * n);

  // CPU reference, same steps as the kernels.
  std::vector<float> work = data, mean(n), stddev(n);
  for (int j = 0; j < n; j++) {
    float sum = 0.0f;
    for (int i = 0; i < m; i++) sum += work[i * n + j];
    mean[j] = sum / m;
  }
  for (int j = 0; j < n; j++) {
    float sum = 0.0f;
    for (int i = 0; i < m; i++) {
      const float d = work[i * n + j] - mean[j];
      sum += d * d;
    }
    const float s = std::sqrt(sum / m);
    stddev[j] = s < 1e-12f ? 1.0f : s;
  }
  const float sqrtM = std::sqrt(static_cast<float>(m));
  for (int idx = 0; idx < m * n; idx++) {
    const int j = idx % n;
    work[idx] = (work[idx] - mean[j]) / (sqrtM * stddev[j]);
  }
  std::vector<double> ref(static_cast<size_t>(n) * n);
  for (int row = 0; row < n; row++) {
    for (int col = 0; col < n; col++) {
      double sum = 0.0;
      for (int k = 0; k < m; k++) sum += static_cast<double>(work[k * n + row]) * work[k * n + col];
      ref[row * n + col] = row == col ? 1.0 : sum;
    }
  }

  int mismatches = countMismatches("corr", ref, corr);
  cudaFree(dData); cudaFree(dMean); cudaFree(dStddev); cudaFree(dCorr);
  return finish(mismatches);
}
