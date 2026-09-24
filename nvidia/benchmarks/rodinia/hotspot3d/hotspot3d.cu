// hotspot3d.cu: CUDA host program for Rodinia HotSpot3D.
//
// Kernel: amd/benchmarks/rodinia/hotspot3d/native/rodinia_hotspot3d.cpp
// Host:   follows amd/benchmarks/rodinia/hotspot3d/hotspot3d.go
//         (one launch per iteration; the grid's z dimension is one plane per block)
//
// Usage: hotspot3d [-size N] [-iterations I] [-amb-temp T]
#include <utility>

#include "check.h"
#include "rodinia/hotspot3d/native/rodinia_hotspot3d.cpp"

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 32);
  const int iters = intArg(argc, argv, "iterations", 2);
  const float ambTemp = floatArg(argc, argv, "amb-temp", 80.0f);
  std::printf("HotSpot3D: N=%d iterations=%d\n", n, iters);

  // Thermal parameters, computed in float as in hotspot3d.go.
  const float chip = 0.016f, tChip = 0.0005f, kSi = 100.0f, cSi = 1.75e6f;
  const float dx = chip / n, dy = chip / n, dz = chip / n;
  const float cap = cSi * tChip * dx * dy;
  const float rx = dx / (2.0f * kSi * tChip * dy);
  const float ry = dy / (2.0f * kSi * tChip * dx);
  const float rz = dz / (2.0f * kSi * tChip * dx);
  const float ra = tChip / (kSi * dx * dy);
  const float step = 0.001f / (kSi / (0.5f * tChip * cSi));
  const float stepDivCap = step / cap;
  const float rx1 = 1.0f / rx, ry1 = 1.0f / ry, rz1 = 1.0f / rz, ra1 = 1.0f / ra;

  const size_t total = static_cast<size_t>(n) * n * n;
  std::vector<float> temp(total), power(total);
  for (size_t i = 0; i < total; i++) {
    temp[i] = ambTemp + static_cast<float>(i % 200) / 10.0f;
    power[i] = static_cast<float>((i * 7) % 100) / 500.0f;
  }

  float* src = toDevice(temp);
  float* dst = deviceAlloc<float>(total);
  float* dPower = toDevice(power);

  const int g = ceilDiv(n, BLOCK_DIM);
  for (int k = 0; k < iters; k++) {
    hotspot3d_kernel<<<dim3(g, g, n), dim3(BLOCK_DIM, BLOCK_DIM, 1)>>>(
        src, dst, dPower, n, n, n, stepDivCap, rx1, ry1, rz1, ra1, ambTemp);
    CHECK_LAUNCH();
    std::swap(src, dst);
  }

  std::vector<float> got = fromDevice(src, total);

  std::vector<float> cur = temp, next(total);
  const int plane = n * n;
  for (int k = 0; k < iters; k++) {
    for (int z = 0; z < n; z++)
      for (int y = 0; y < n; y++)
        for (int x = 0; x < n; x++) {
          const int idx = z * plane + y * n + x;
          const float tc = cur[idx];
          const float txm = x > 0 ? cur[idx - 1] : tc, txp = x < n - 1 ? cur[idx + 1] : tc;
          const float tym = y > 0 ? cur[idx - n] : tc, typ = y < n - 1 ? cur[idx + n] : tc;
          const float tzm = z > 0 ? cur[idx - plane] : tc, tzp = z < n - 1 ? cur[idx + plane] : tc;
          next[idx] = tc + stepDivCap * (power[idx] + (txm + txp - 2.0f * tc) * rx1 +
                                         (tym + typ - 2.0f * tc) * ry1 +
                                         (tzm + tzp - 2.0f * tc) * rz1 + (ambTemp - tc) * ra1);
        }
    std::swap(cur, next);
  }

  int mismatches = countMismatches("temp", cur, got);
  cudaFree(src); cudaFree(dst); cudaFree(dPower);
  return finish(mismatches);
}
