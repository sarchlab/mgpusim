# Rodinia PathFinder

CUDA host program `pathfinder.cu` for the Rodinia PathFinder benchmark. The GPU kernels are not copied: `pathfinder.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/rodinia/pathfinder/native/rodinia_pathfinder.cpp` |
| Kernel version | commit `146a4c8c` (2026-06-17) on main |
| Kernel origin | Extracted from `sarchlab/gpu_benchmarks` tier2/rodinia_pathfinder |
| Kernels | `dynproc_kernel` |
| Host code follows | `amd/benchmarks/rodinia/pathfinder/pathfinder.go` |
| Host code version | commit `90b1b660` (2026-06-25) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/pathfinder` |
| Usage | `pathfinder [-rows ROWS] [-cols COLS]` |

## Differences from the AMD host code

None beyond the common ones below.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
