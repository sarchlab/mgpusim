// pathfinder.cu: CUDA host program for Rodinia PathFinder.
//
// Kernel: amd/benchmarks/rodinia/pathfinder/native/rodinia_pathfinder.cpp
// Host:   follows amd/benchmarks/rodinia/pathfinder/pathfinder.go
//         (one launch per grid row, ping-ponging two row buffers)
//
// Usage: pathfinder [-rows ROWS] [-cols COLS]
#include <utility>

#include "check.h"
#include "rodinia/pathfinder/native/rodinia_pathfinder.cpp"

int main(int argc, char** argv) {
  const int rows = intArg(argc, argv, "rows", 64);
  const int cols = intArg(argc, argv, "cols", 128);
  std::printf("PathFinder: rows=%d cols=%d\n", rows, cols);

  // Same pseudo-random wall as genWall in pathfinder.go.
  std::vector<int> wall(static_cast<size_t>(rows) * cols);
  for (size_t i = 0; i < wall.size(); i++)
    wall[i] = static_cast<int>(((i * 1103515245 + 12345) & 0x7fffffff) % 10);

  int* dWall = toDevice(wall);
  int* src = toDevice(std::vector<int>(wall.begin(), wall.begin() + cols));
  int* dst = deviceAlloc<int>(cols);

  for (int t = 1; t < rows; t++) {
    dynproc_kernel<<<ceilDiv(cols, BLOCK_SIZE), BLOCK_SIZE>>>(dWall, src, dst, cols, t);
    CHECK_LAUNCH();
    std::swap(src, dst);
  }

  std::vector<int> got = fromDevice(src, cols);

  std::vector<int> cur(wall.begin(), wall.begin() + cols), next(cols);
  for (int t = 1; t < rows; t++) {
    for (int c = 0; c < cols; c++) {
      int best = cur[c];
      if (c > 0) best = std::min(best, cur[c - 1]);
      if (c < cols - 1) best = std::min(best, cur[c + 1]);
      next[c] = wall[static_cast<size_t>(t) * cols + c] + best;
    }
    std::swap(cur, next);
  }

  int mismatches = 0;
  for (int c = 0; c < cols; c++) mismatches += cur[c] != got[c];
  cudaFree(dWall); cudaFree(src); cudaFree(dst);
  return finish(mismatches);
}
