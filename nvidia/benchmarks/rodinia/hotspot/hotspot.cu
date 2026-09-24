// hotspot.cu: CUDA host program for Rodinia HotSpot (2D thermal simulation).
//
// Kernel: amd/benchmarks/rodinia/hotspot/native/rodinia_hotspot.cpp
// Host:   follows amd/benchmarks/rodinia/hotspot/hotspot.go
//         (one launch per iteration, ping-ponging two temperature grids)
//
// Usage: hotspot [-size N] [-iterations I]
#include <utility>

#include "check.h"
#include "rodinia/hotspot/native/rodinia_hotspot.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 32);
  const int iters = intArg(argc, argv, "iterations", 10);
  std::printf("HotSpot: N=%d iterations=%d\n", n, iters);

  // Thermal parameters, computed in double as in hotspot.go.
  const double chipHeight = 0.016, chipWidth = 0.016, tChip = 0.0005;
  const double kSi = 100.0, cSi = 1.75e6;
  const double gh = chipHeight / n, gw = chipWidth / n;
  const double cap = cSi * tChip * gh * gw;
  const double rx = gw / (2.0 * kSi * tChip * gh);
  const double ry = gh / (2.0 * kSi * tChip * gw);
  const double rz = tChip / (kSi * gh * gw);
  const double step = 0.001 / (kSi / (0.5 * tChip * cSi));
  const float stepDivCap = static_cast<float>(step / cap);
  const float rx1 = static_cast<float>(1.0 / rx), ry1 = static_cast<float>(1.0 / ry),
              rz1 = static_cast<float>(1.0 / rz);

  const size_t total = static_cast<size_t>(n) * n;
  std::vector<float> temp(total), power(total);
  for (size_t i = 0; i < total; i++) {
    temp[i] = AMB_TEMP + static_cast<float>(i % 200) / 10.0f;
    power[i] = static_cast<float>(i % 100) / 500.0f;
  }

  float* src = toDevice(temp);
  float* dst = deviceAlloc<float>(total);
  float* dPower = toDevice(power);

  const int g = ceilDiv(n, BLOCK_SIZE);
  for (int k = 0; k < iters; k++) {
    hotspot_kernel<<<dim3(g, g), dim3(BLOCK_SIZE, BLOCK_SIZE)>>>(
        src, dst, dPower, n, n, stepDivCap, rx1, ry1, rz1);
    CHECK_LAUNCH();
    std::swap(src, dst);
  }

  std::vector<float> got = fromDevice(src, total);

  std::vector<float> cur = temp, next(total);
  for (int k = 0; k < iters; k++) {
    for (int row = 0; row < n; row++)
      for (int col = 0; col < n; col++) {
        const int idx = row * n + col;
        const float tc = cur[idx];
        const float tn = row > 0 ? cur[idx - n] : tc;
        const float ts = row < n - 1 ? cur[idx + n] : tc;
        const float tw = col > 0 ? cur[idx - 1] : tc;
        const float te = col < n - 1 ? cur[idx + 1] : tc;
        next[idx] = tc + stepDivCap * (power[idx] + (tn + ts - 2.0f * tc) * ry1 +
                                       (tw + te - 2.0f * tc) * rx1 + (AMB_TEMP - tc) * rz1);
      }
    std::swap(cur, next);
  }

  int mismatches = countMismatches("temp", cur, got);
  cudaFree(src); cudaFree(dst); cudaFree(dPower);
  return finish(mismatches);
}
