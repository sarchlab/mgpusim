// check.h: small helpers shared by the CUDA host programs.
#ifndef MGPUSIM_NVIDIA_CHECK_H
#define MGPUSIM_NVIDIA_CHECK_H

#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>

#include <cuda_runtime.h>

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

#define CHECK_CUDA(call)                                                  \
  do {                                                                    \
    cudaError_t err_ = (call);                                            \
    if (err_ != cudaSuccess) {                                            \
      std::fprintf(stderr, "%s:%d: %s failed: %s\n", __FILE__, __LINE__, \
                   #call, cudaGetErrorString(err_));                      \
      std::exit(1);                                                       \
    }                                                                     \
  } while (0)

// CHECK_LAUNCH reports errors from the most recent kernel launch.
#define CHECK_LAUNCH() CHECK_CUDA(cudaGetLastError())

// intArg returns the value of "-name <int>" on the command line, or def.
static inline int intArg(int argc, char** argv, const char* name, int def) {
  for (int i = 1; i + 1 < argc; i++) {
    if (argv[i][0] == '-' && std::strcmp(argv[i] + 1, name) == 0) {
      return std::atoi(argv[i + 1]);
    }
  }
  return def;
}

// floatArg returns the value of "-name <float>" on the command line, or def.
static inline float floatArg(int argc, char** argv, const char* name, float def) {
  for (int i = 1; i + 1 < argc; i++) {
    if (argv[i][0] == '-' && std::strcmp(argv[i] + 1, name) == 0) {
      return static_cast<float>(std::atof(argv[i + 1]));
    }
  }
  return def;
}

// ceilDiv returns the number of blocks of size b needed to cover n.
static inline int ceilDiv(int n, int b) { return (n + b - 1) / b; }

// deviceAlloc allocates n elements of T on the device, zero-initialized.
template <class T>
static T* deviceAlloc(size_t n) {
  T* p = nullptr;
  CHECK_CUDA(cudaMalloc(&p, n * sizeof(T)));
  CHECK_CUDA(cudaMemset(p, 0, n * sizeof(T)));
  return p;
}

// toDevice allocates a device buffer and copies v into it.
template <class T>
static T* toDevice(const std::vector<T>& v) {
  T* p = deviceAlloc<T>(v.size());
  CHECK_CUDA(cudaMemcpy(p, v.data(), v.size() * sizeof(T),
                        cudaMemcpyHostToDevice));
  return p;
}

// copyToDevice copies v into an existing device buffer.
template <class T>
static void copyToDevice(T* p, const std::vector<T>& v) {
  CHECK_CUDA(cudaMemcpy(p, v.data(), v.size() * sizeof(T),
                        cudaMemcpyHostToDevice));
}

// fromDevice copies n elements from the device.
template <class T>
static std::vector<T> fromDevice(const T* p, size_t n) {
  std::vector<T> v(n);
  CHECK_CUDA(cudaMemcpy(v.data(), p, n * sizeof(T), cudaMemcpyDeviceToHost));
  return v;
}

// Mismatch counting uses the rule of the AMD Verify functions:
// |ref - got| / max(|ref|, floor) must not exceed tol.
static inline bool isClose(double ref, double got, double tol = 1e-3,
                           double floor = 1.0) {
  double denom = std::fabs(ref);
  if (denom < floor) denom = floor;
  return std::fabs(ref - got) / denom <= tol;
}

template <class R, class G>
static int countMismatches(const char* name, const std::vector<R>& ref,
                           const std::vector<G>& got, double tol = 1e-3,
                           double floor = 1.0) {
  int mismatches = 0;
  for (size_t i = 0; i < ref.size(); i++) {
    if (!isClose(static_cast<double>(ref[i]), static_cast<double>(got[i]),
                 tol, floor)) {
      if (mismatches < 5) {
        std::printf("%s mismatch at %zu: expected %f, got %f\n", name, i,
                    static_cast<double>(ref[i]), static_cast<double>(got[i]));
      }
      mismatches++;
    }
  }
  return mismatches;
}

// finish prints the verification result and returns the process exit code.
static inline int finish(int mismatches) {
  if (mismatches > 0) {
    std::printf("Failed: %d mismatches\n", mismatches);
    return 1;
  }
  std::printf("Passed!\n");
  return 0;
}

#endif  // MGPUSIM_NVIDIA_CHECK_H
