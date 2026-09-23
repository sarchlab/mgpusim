// conv3d.cu: CUDA host program for PolyBench 3D convolution.
//
// Kernel: amd/benchmarks/polybench/conv3d/native/polybench_3dconv.cpp
// Host:   follows amd/benchmarks/polybench/conv3d/conv3d.go
//
// Usage: conv3d [-size N] [-filter-size F]
#include "check.h"
#include "polybench/conv3d/native/polybench_3dconv.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 32);
  const int fs = intArg(argc, argv, "filter-size", 3);
  std::printf("3DCONV: N=%d filter=%d\n", n, fs);

  const size_t vol = static_cast<size_t>(n) * n * n;
  std::vector<float> input(vol), filter(static_cast<size_t>(fs) * fs * fs);
  for (size_t i = 0; i < vol; i++) input[i] = static_cast<float>(i % 100) / 100.0f;
  float filtSum = 0.0f;
  for (size_t i = 0; i < filter.size(); i++) {
    filter[i] = static_cast<float>(i % 10 + 1);
    filtSum += filter[i];
  }
  for (float& f : filter) f /= filtSum;

  float* dIn = toDevice(input);
  float* dFilter = toDevice(filter);
  float* dOut = deviceAlloc<float>(vol);

  const int g = ceilDiv(n, BLOCK_SIZE);
  conv3d_kernel<<<dim3(g, g, g), dim3(BLOCK_SIZE, BLOCK_SIZE, BLOCK_SIZE)>>>(
      dIn, dFilter, dOut, n, fs);
  CHECK_LAUNCH();

  std::vector<float> out = fromDevice(dOut, vol);

  const int half = fs / 2;
  std::vector<double> ref(vol, 0.0);
  for (int i = 0; i < n; i++) {
    for (int j = 0; j < n; j++) {
      for (int k = 0; k < n; k++) {
        double sum = 0.0;
        for (int fi = 0; fi < fs; fi++) {
          const int ii = i - half + fi;
          if (ii < 0 || ii >= n) continue;
          for (int fj = 0; fj < fs; fj++) {
            const int jj = j - half + fj;
            if (jj < 0 || jj >= n) continue;
            for (int fk = 0; fk < fs; fk++) {
              const int kk = k - half + fk;
              if (kk < 0 || kk >= n) continue;
              sum += static_cast<double>(input[(static_cast<size_t>(ii) * n + jj) * n + kk]) *
                     filter[(fi * fs + fj) * fs + fk];
            }
          }
        }
        ref[(static_cast<size_t>(i) * n + j) * n + k] = sum;
      }
    }
  }

  int mismatches = countMismatches("output", ref, out);
  cudaFree(dIn); cudaFree(dFilter); cudaFree(dOut);
  return finish(mismatches);
}
