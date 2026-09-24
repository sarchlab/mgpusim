// lavamd.cu: CUDA host program for Rodinia LavaMD (Lennard-Jones forces
// between particles in neighboring boxes).
//
// Kernel: amd/benchmarks/rodinia/lavamd/native/rodinia_lavamd.cpp
// Host:   follows amd/benchmarks/rodinia/lavamd/lavamd.go
//         (one thread block per box)
//
// Usage: lavamd [-num-boxes B] [-particles-per-box P]
#include "check.h"
#include "rodinia/lavamd/native/rodinia_lavamd.cpp"

static const float kBoxSize = 10.0f;

static unsigned lcg(unsigned* s) { return *s = *s * 1664525u + 1013904223u; }
static float randFloat(unsigned* s, float lo, float hi) {
  return lo + static_cast<float>(lcg(s) & 0xFFFF) / 65535.0f * (hi - lo);
}

int main(int argc, char** argv) {
  const int nb = intArg(argc, argv, "num-boxes", 4);
  const int ppb = intArg(argc, argv, "particles-per-box", 100);
  const int totalBoxes = nb * nb * nb;
  const int total = totalBoxes * ppb;
  std::printf("LavaMD: boxes=%d^3 particles/box=%d\n", nb, ppb);

  std::vector<float> px(total), py(total), pz(total);
  std::vector<int> nlist(totalBoxes * 27, 0), ncount(totalBoxes, 0);
  unsigned seed = 42;
  for (int bz = 0; bz < nb; bz++)
    for (int by = 0; by < nb; by++)
      for (int bx = 0; bx < nb; bx++) {
        const int box = (bz * nb + by) * nb + bx;
        for (int p = 0; p < ppb; p++) {
          px[box * ppb + p] = bx * kBoxSize + randFloat(&seed, 0.5f, kBoxSize - 0.5f);
          py[box * ppb + p] = by * kBoxSize + randFloat(&seed, 0.5f, kBoxSize - 0.5f);
          pz[box * ppb + p] = bz * kBoxSize + randFloat(&seed, 0.5f, kBoxSize - 0.5f);
        }
        int count = 0;
        for (int dz = -1; dz <= 1; dz++)
          for (int dy = -1; dy <= 1; dy++)
            for (int dx = -1; dx <= 1; dx++) {
              const int x = bx + dx, y = by + dy, z = bz + dz;
              if (x >= 0 && x < nb && y >= 0 && y < nb && z >= 0 && z < nb)
                nlist[box * 27 + count++] = (z * nb + y) * nb + x;
            }
        ncount[box] = count;
      }

  float *dPX = toDevice(px), *dPY = toDevice(py), *dPZ = toDevice(pz);
  float *dFX = deviceAlloc<float>(total), *dFY = deviceAlloc<float>(total),
        *dFZ = deviceAlloc<float>(total), *dE = deviceAlloc<float>(total);
  int *dList = toDevice(nlist), *dCount = toDevice(ncount);

  lavamd_kernel<<<totalBoxes, BLOCK_SIZE>>>(dPX, dPY, dPZ, dFX, dFY, dFZ, dE, dList, dCount,
                                            ppb, totalBoxes);
  CHECK_LAUNCH();

  std::vector<float> fx = fromDevice(dFX, total), fy = fromDevice(dFY, total),
                     fz = fromDevice(dFZ, total), pe = fromDevice(dE, total);

  std::vector<float> rfx(total), rfy(total), rfz(total), rpe(total);
  for (int box = 0; box < totalBoxes; box++)
    for (int p = 0; p < ppb; p++) {
      const int i = box * ppb + p;
      float ax = 0, ay = 0, az = 0, e = 0;
      for (int nn = 0; nn < ncount[box]; nn++)
        for (int q = 0; q < ppb; q++) {
          const int j = nlist[box * 27 + nn] * ppb + q;
          const float dx = px[i] - px[j], dy = py[i] - py[j], dz = pz[i] - pz[j];
          const float r2 = dx * dx + dy * dy + dz * dz;
          if (r2 > 1e-10f) {
            const float r2inv = 1.0f / r2, r6inv = r2inv * r2inv * r2inv;
            const float force = r2inv * r6inv * (LJ_A * r6inv - LJ_B);
            e += r6inv * (LJ_A * r6inv - LJ_B);
            ax += force * dx;
            ay += force * dy;
            az += force * dz;
          }
        }
      rfx[i] = ax; rfy[i] = ay; rfz[i] = az; rpe[i] = e;
    }

  int mismatches = countMismatches("force_x", rfx, fx) + countMismatches("force_y", rfy, fy) +
                   countMismatches("force_z", rfz, fz) + countMismatches("energy", rpe, pe);
  cudaFree(dPX); cudaFree(dPY); cudaFree(dPZ); cudaFree(dFX); cudaFree(dFY);
  cudaFree(dFZ); cudaFree(dE); cudaFree(dList); cudaFree(dCount);
  return finish(mismatches);
}
