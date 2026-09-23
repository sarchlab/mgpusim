# PolyBench/GPU FDTD-2D

CUDA host program `fdtd2d.cu` for the PolyBench/GPU FDTD-2D benchmark. The GPU kernels are not copied: `fdtd2d.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/polybench/fdtd2d/native/polybench_fdtd2d.cpp` |
| Kernel version | commit `146a4c8c` (2026-06-17) on main |
| Kernel origin | Extracted from `sarchlab/gpu_benchmarks` tier2/polybench_fdtd2d |
| Kernels | `fdtd_update_ex`, `fdtd_update_ey`, `fdtd_update_hz` |
| Host code follows | `amd/benchmarks/polybench/fdtd2d/fdtd2d.go` |
| Host code version | commit `90b1b660` (2026-06-25) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/fdtd2d` |
| Usage | `fdtd2d [-size N] [-tmax T]` |

## Differences from the AMD host code

None beyond the common ones below.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
