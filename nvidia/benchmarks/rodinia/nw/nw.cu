// nw.cu: CUDA host program for Rodinia Needleman-Wunsch (sequence alignment).
//
// Kernels: amd/benchmarks/rodinia/nw/native/nw.cpp
// Host:    follows amd/benchmarks/rodinia/nw/benchmark.go
//          (block size 64; nw_kernel1 sweeps the upper-left triangle of
//          blocks, nw_kernel2 the lower-right one)
//
// Usage: nw [-length L] [-penalty P]   (L must be a multiple of 64)
#include "check.h"
#include "rodinia/nw/native/nw.cpp"

static const int kBlockSize = 64;  // nw.cpp sizes its shared arrays for 64

// BLOSUM62, copied from amd/benchmarks/rodinia/nw/benchmark.go.
static const int kBlosum62[24][24] = {
    {4, -1, -2, -2, 0, -1, -1, 0, -2, -1, -1, -1, -1, -2, -1, 1, 0, -3, -2, 0, -2, -1, 0, -4},
    {-1, 5, 0, -2, -3, 1, 0, -2, 0, -3, -2, 2, -1, -3, -2, -1, -1, -3, -2, -3, -1, 0, -1, -4},
    {-2, 0, 6, 1, -3, 0, 0, 0, 1, -3, -3, 0, -2, -3, -2, 1, 0, -4, -2, -3, 3, 0, -1, -4},
    {-2, -2, 1, 6, -3, 0, 2, -1, -1, -3, -4, -1, -3, -3, -1, 0, -1, -4, -3, -3, 4, 1, -1, -4},
    {0, -3, -3, -3, 9, -3, -4, -3, -3, -1, -1, -3, -1, -2, -3, -1, -1, -2, -2, -1, -3, -3, -2, -4},
    {-1, 1, 0, 0, -3, 5, 2, -2, 0, -3, -2, 1, 0, -3, -1, 0, -1, -2, -1, -2, 0, 3, -1, -4},
    {-1, 0, 0, 2, -4, 2, 5, -2, 0, -3, -3, 1, -2, -3, -1, 0, -1, -3, -2, -2, 1, 4, -1, -4},
    {0, -2, 0, -1, -3, -2, -2, 6, -2, -4, -4, -2, -3, -3, -2, 0, -2, -2, -3, -3, -1, -2, -1, -4},
    {-2, 0, 1, -1, -3, 0, 0, -2, 8, -3, -3, -1, -2, -1, -2, -1, -2, -2, 2, -3, 0, 0, -1, -4},
    {-1, -3, -3, -3, -1, -3, -3, -4, -3, 4, 2, -3, 1, 0, -3, -2, -1, -3, -1, 3, -3, -3, -1, -4},
    {-1, -2, -3, -4, -1, -2, -3, -4, -3, 2, 4, -2, 2, 0, -3, -2, -1, -2, -1, 1, -4, -3, -1, -4},
    {-1, 2, 0, -1, -3, 1, 1, -2, -1, -3, -2, 5, -1, -3, -1, 0, -1, -3, -2, -2, 0, 1, -1, -4},
    {-1, -1, -2, -3, -1, 0, -2, -3, -2, 1, 2, -1, 5, 0, -2, -1, -1, -1, -1, 1, -3, -1, -4},
    {-2, -3, -3, -3, -2, -3, -3, -3, -1, 0, 0, -3, 0, 6, -4, -2, -2, 1, 3, -1, -3, -3, -1, -4},
    {-1, -2, -2, -1, -3, -1, -1, -2, -2, -3, -3, -1, -2, -4, 7, -1, -1, -4, -3, -2, -2, -1, -2, -4},
    {1, -1, 1, 0, -1, 0, 0, 0, -1, -2, -2, 0, -1, -2, -1, 4, 1, -3, -2, -2, 0, 0, 0, -4},
    {0, -1, 0, -1, -1, -1, -1, -2, -2, -1, -1, -1, -1, -2, -1, 1, 5, -2, -2, 0, -1, -1, 0, -4},
    {-3, -3, -4, -4, -2, -2, -3, -2, -2, -3, -2, -3, -1, 1, -4, -3, -2, 11, 2, -3, -4, -3, -2, -4},
    {-2, -2, -2, -3, -2, -1, -2, -3, 2, -1, -1, -2, -1, 3, -3, -2, -2, 2, 7, -1, -3, -2, -1, -4},
    {0, -3, -3, -3, -1, -2, -2, -3, -3, 3, 1, -2, 1, -1, -2, -2, 0, -3, -1, 4, -3, -2, -1, -4},
    {-2, -1, 3, 4, -3, 0, 1, -1, 0, -3, -4, 0, -3, -3, -2, 0, -1, -4, -3, -3, 4, 1, -1, -4},
    {-1, 0, 0, 1, -3, 3, 4, -2, 0, -3, -3, 1, -1, -3, -1, 0, -1, -3, -2, -2, 1, 4, -1, -4},
    {0, -1, -1, -1, -2, -1, -1, -1, -1, -1, -1, -1, -1, -1, -2, 0, 0, -2, -1, -1, -1, -1, -1, -4},
    {-4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, 1},
};

int main(int argc, char** argv) {
  const int length = intArg(argc, argv, "length", 64);
  const int penalty = intArg(argc, argv, "penalty", 10);
  if (length % kBlockSize != 0) {
    std::fprintf(stderr, "length must be a multiple of %d\n", kBlockSize);
    return 2;
  }
  const int rows = length + 1, cols = length + 1;
  std::printf("NW: length=%d penalty=%d\n", length, penalty);

  // Random sequences in 1..10 (a fixed-seed LCG instead of Go's math/rand).
  unsigned seed = 1;
  auto rnd = [&seed]() {
    seed = seed * 1103515245u + 12345u;
    return static_cast<int>((seed >> 16) & 0x7fff);
  };
  std::vector<int> items(static_cast<size_t>(rows) * cols, 0), ref(items.size(), 0);
  for (int i = 0; i < rows; i++) items[i * cols] = rnd() % 10 + 1;
  for (int j = 0; j < cols; j++) items[j] = rnd() % 10 + 1;
  for (int i = 0; i < rows; i++)
    for (int j = 0; j < cols; j++) ref[i * cols + j] = kBlosum62[items[i * cols]][items[j]];
  items[0] = 0;
  for (int i = 1; i < rows; i++) items[i * cols] = -i * penalty;
  for (int j = 1; j < cols; j++) items[j] = -j * penalty;

  int* dRef = toDevice(ref);
  int* dItems = toDevice(items);
  int* dOut = deviceAlloc<int>(items.size());

  const int workSize = cols - 1;
  const int blockWidth = workSize / kBlockSize;
  for (int blk = 1; blk <= blockWidth; blk++) {
    nw_kernel1<<<blk, kBlockSize>>>(dRef, dItems, dOut, cols, penalty, blk, kBlockSize,
                                    blockWidth, workSize, 0, 0);
    CHECK_LAUNCH();
  }
  // Rodinia sweeps the lower-right triangle from the longest anti-diagonal
  // down. benchmark.go counts blk upward instead, which only gives the same
  // result when there is a single block (length 64).
  for (int blk = blockWidth - 1; blk >= 1; blk--) {
    nw_kernel2<<<blk, kBlockSize>>>(dRef, dItems, dOut, cols, penalty, blk, kBlockSize,
                                    blockWidth, workSize, 0, 0);
    CHECK_LAUNCH();
  }

  std::vector<int> got = fromDevice(dItems, items.size());

  for (int i = 1; i < rows; i++)
    for (int j = 1; j < cols; j++) {
      const int left = items[i * cols + j - 1] - penalty;
      const int top = items[(i - 1) * cols + j] - penalty;
      const int diag = items[(i - 1) * cols + j - 1] + ref[i * cols + j];
      items[i * cols + j] = std::max(left, std::max(top, diag));
    }

  int mismatches = 0;
  for (size_t i = 0; i < items.size(); i++) mismatches += items[i] != got[i];
  cudaFree(dRef); cudaFree(dItems); cudaFree(dOut);
  return finish(mismatches);
}
