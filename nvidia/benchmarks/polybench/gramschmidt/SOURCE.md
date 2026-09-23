# PolyBench/GPU GRAMSCHM

CUDA host program `gramschmidt.cu` for the PolyBench/GPU GRAMSCHM benchmark. The GPU kernels are not copied: `gramschmidt.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/polybench/gramschmidt/native/polybench_gramschmidt.cpp` |
| Kernel version | commit `146a4c8c` (2026-06-17) on main |
| Kernel origin | Extracted from `sarchlab/gpu_benchmarks` tier2/polybench_gramschmidt |
| Kernels | `gram_norm_finish`, `gram_normalize`, `gram_project` |
| Host code follows | `amd/benchmarks/polybench/gramschmidt/gramschmidt.go` |
| Host code version | commit `90b1b660` (2026-06-25) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/gramschmidt` |
| Usage | `gramschmidt [-m M] [-n N]` |

## Differences from the AMD host code

- The extra orthogonality check on the first 8 columns of Q in `gramschmidt.go` is not repeated; Q and R are compared element-wise as in the Go code.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
