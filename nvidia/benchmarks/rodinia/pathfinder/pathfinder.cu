// pathfinder.cu: CUDA host program for Rodinia PathFinder.
//
// The kernel is not copied here. It is compiled from the same source as the
// AMD benchmark, amd/benchmarks/rodinia/pathfinder/native/rodinia_pathfinder.cpp,
// and this host program follows amd/benchmarks/rodinia/pathfinder/pathfinder.go:
// one kernel launch per grid row, ping-ponging between two row buffers.
//
// Usage: pathfinder [-rows ROWS] [-cols COLS]
#include <algorithm>
#include <climits>
#include <cstdio>
#include <vector>

#include "check.h"
#include "rodinia/pathfinder/native/rodinia_pathfinder.cpp"

int main(int argc, char** argv) {
  const int rows = intArg(argc, argv, "rows", 64);
  const int cols = intArg(argc, argv, "cols", 128);
  std::printf("PathFinder: rows=%d cols=%d\n", rows, cols);

  std::vector<int> wall(static_cast<size_t>(rows) * cols);
  // Same pseudo-random wall as genWall in pathfinder.go.
  for (size_t i = 0; i < wall.size(); i++) {
    wall[i] = static_cast<int>(((i * 1103515245 + 12345) & 0x7fffffff) % 10);
  }

  int *dWall, *dBuf[2];
  CHECK_CUDA(cudaMalloc(&dWall, wall.size() * sizeof(int)));
  CHECK_CUDA(cudaMalloc(&dBuf[0], cols * sizeof(int)));
  CHECK_CUDA(cudaMalloc(&dBuf[1], cols * sizeof(int)));
  CHECK_CUDA(cudaMemcpy(dWall, wall.data(), wall.size() * sizeof(int),
                        cudaMemcpyHostToDevice));
  CHECK_CUDA(cudaMemcpy(dBuf[0], wall.data(), cols * sizeof(int),
                        cudaMemcpyHostToDevice));

  const int numBlocks = (cols + BLOCK_SIZE - 1) / BLOCK_SIZE;
  int src = 0;
  for (int t = 1; t < rows; t++) {
    const int dst = 1 - src;
    dynproc_kernel<<<numBlocks, BLOCK_SIZE>>>(dWall, dBuf[src], dBuf[dst],
                                            cols, t);
    CHECK_CUDA(cudaGetLastError());
    src = dst;
  }

  std::vector<int> result(cols);
  CHECK_CUDA(cudaMemcpy(result.data(), dBuf[src], cols * sizeof(int),
                        cudaMemcpyDeviceToHost));

  // CPU reference.
  std::vector<int> cur(wall.begin(), wall.begin() + cols);
  std::vector<int> next(cols);
  for (int t = 1; t < rows; t++) {
    for (int c = 0; c < cols; c++) {
      int best = cur[c];
      if (c > 0) best = std::min(best, cur[c - 1]);
      if (c < cols - 1) best = std::min(best, cur[c + 1]);
      next[c] = wall[static_cast<size_t>(t) * cols + c] + best;
    }
    cur.swap(next);
  }

  CHECK_CUDA(cudaFree(dWall));
  CHECK_CUDA(cudaFree(dBuf[0]));
  CHECK_CUDA(cudaFree(dBuf[1]));

  for (int c = 0; c < cols; c++) {
    if (result[c] != cur[c]) {
      std::printf("Failed: column %d, expected %d, got %d\n", c, cur[c],
                  result[c]);
      return 1;
    }
  }
  std::printf("Passed!\n");
  return 0;
}
