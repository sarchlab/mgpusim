// srad.cu: CUDA host program for Rodinia SRAD (speckle-reducing anisotropic
// diffusion).
//
// Kernels: amd/benchmarks/rodinia/srad/native/rodinia_srad.cpp
// Host:    follows amd/benchmarks/rodinia/srad/srad.go
//          (srad1 + srad2 per iteration)
//
// Usage: srad [-size N] [-iterations I]
#include "check.h"
#include "rodinia/srad/native/rodinia_srad.cpp"

static const float kLambda = 0.25f;
static const float kQ0sqr = 0.05f;

int main(int argc, char** argv) {
  const int n = intArg(argc, argv, "size", 32);
  const int iters = intArg(argc, argv, "iterations", 10);
  std::printf("SRAD: N=%d iterations=%d\n", n, iters);

  const size_t total = static_cast<size_t>(n) * n;
  std::vector<float> jInit(total);
  for (size_t i = 0; i < total; i++) jInit[i] = static_cast<float>(i % 97 + 1) / 98.0f;

  float* dJ = toDevice(jInit);
  float *dN = deviceAlloc<float>(total), *dS = deviceAlloc<float>(total),
        *dW = deviceAlloc<float>(total), *dE = deviceAlloc<float>(total),
        *dC = deviceAlloc<float>(total);

  const int g = ceilDiv(n, BLOCK_SIZE);
  const dim3 grid(g, g), block(BLOCK_SIZE, BLOCK_SIZE);
  for (int k = 0; k < iters; k++) {
    srad1<<<grid, block>>>(dJ, dN, dS, dW, dE, dC, n, n, kQ0sqr);
    CHECK_LAUNCH();
    srad2<<<grid, block>>>(dJ, dN, dS, dW, dE, dC, n, n, kLambda);
    CHECK_LAUNCH();
  }

  std::vector<float> got = fromDevice(dJ, total);

  std::vector<float> j = jInit, vn(total), vs(total), vw(total), ve(total), c(total);
  for (int k = 0; k < iters; k++) {
    for (int row = 0; row < n; row++)
      for (int col = 0; col < n; col++) {
        const int idx = row * n + col;
        const int iN = row > 0 ? row - 1 : 0, iS = row < n - 1 ? row + 1 : n - 1;
        const int jW = col > 0 ? col - 1 : 0, jE = col < n - 1 ? col + 1 : n - 1;
        const float jc = j[idx];
        const float dn = j[iN * n + col] - jc, ds = j[iS * n + col] - jc;
        const float dw = j[row * n + jW] - jc, de = j[row * n + jE] - jc;
        vn[idx] = dn; vs[idx] = ds; vw[idx] = dw; ve[idx] = de;
        const float g2 = (dn * dn + ds * ds + dw * dw + de * de) / (jc * jc);
        const float l = (dn + ds + dw + de) / jc;
        const float num = 0.5f * g2 - (1.0f / 16.0f) * (l * l);
        float den = 1.0f + 0.25f * l;
        const float qsqr = num / (den * den);
        den = (qsqr - kQ0sqr) / (kQ0sqr * (1.0f + kQ0sqr));
        c[idx] = std::min(1.0f, std::max(0.0f, 1.0f / (1.0f + den)));
      }
    for (int row = 0; row < n; row++)
      for (int col = 0; col < n; col++) {
        const int idx = row * n + col;
        const int iS = row < n - 1 ? row + 1 : n - 1, jE = col < n - 1 ? col + 1 : n - 1;
        const float d = c[idx] * vn[idx] + c[iS * n + col] * vs[idx] + c[idx] * vw[idx] +
                        c[row * n + jE] * ve[idx];
        j[idx] += 0.25f * kLambda * d;
      }
  }

  int mismatches = countMismatches("J", j, got);
  cudaFree(dJ); cudaFree(dN); cudaFree(dS); cudaFree(dW); cudaFree(dE); cudaFree(dC);
  return finish(mismatches);
}
