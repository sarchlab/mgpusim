# PolyBench/GPU BICG

CUDA host program `bicg.cu` for the PolyBench/GPU BICG benchmark. The GPU kernels are not copied: `bicg.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/polybench/bicg/native/bicg.cpp` |
| Kernel version | commit `2d04fc56` (2026-03-05) on main |
| Kernel origin | Translated from bicg.cl for gfx942 CDNA3 architecture |
| Kernels | `bicgKernel1`, `bicgKernel2` |
| Host code follows | `amd/benchmarks/polybench/bicg/benchmark.go` |
| Host code version | commit `7d8578f7` (2026-06-15) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/bicg` |
| Usage | `bicg [-x NX] [-y NY]` |

## Differences from the AMD host code

- Default size is 1024 x 1024 instead of the sample's 4096 x 4096, to keep traces small.
- `benchmark.go` requires bit-exact floats; here a 1e-3 relative tolerance is used because nvcc may contract multiply-adds into FMAs.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
