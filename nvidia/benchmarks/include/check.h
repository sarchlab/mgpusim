// check.h: small helpers shared by the CUDA host programs.
#ifndef MGPUSIM_NVIDIA_CHECK_H
#define MGPUSIM_NVIDIA_CHECK_H

#include <cstdio>
#include <cstdlib>
#include <cstring>

#include <cuda_runtime.h>

#define CHECK_CUDA(call)                                                  \
  do {                                                                    \
    cudaError_t err_ = (call);                                            \
    if (err_ != cudaSuccess) {                                            \
      std::fprintf(stderr, "%s:%d: %s failed: %s\n", __FILE__, __LINE__, \
                   #call, cudaGetErrorString(err_));                      \
      std::exit(1);                                                       \
    }                                                                     \
  } while (0)

// intArg returns the value of "-name <int>" on the command line, or def.
static int intArg(int argc, char** argv, const char* name, int def) {
  for (int i = 1; i + 1 < argc; i++) {
    if (argv[i][0] == '-' && std::strcmp(argv[i] + 1, name) == 0) {
      return std::atoi(argv[i + 1]);
    }
  }
  return def;
}

#endif  // MGPUSIM_NVIDIA_CHECK_H
