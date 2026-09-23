# Rodinia HotSpot3D

CUDA host program `hotspot3d.cu` for the Rodinia HotSpot3D benchmark. The GPU kernels are not copied: `hotspot3d.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/rodinia/hotspot3d/native/rodinia_hotspot3d.cpp` |
| Kernel version | commit `7155c64f` (2026-07-04) on main |
| Kernel origin | Extracted from `sarchlab/gpu_benchmarks` tier2/rodinia_hotspot3d |
| Kernels | `hotspot3d_kernel` |
| Host code follows | `amd/benchmarks/rodinia/hotspot3d/hotspot3d.go` |
| Host code version | commit `7155c64f` (2026-07-04) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/hotspot3d` |
| Usage | `hotspot3d [-size N] [-iterations I] [-amb-temp T]` |

## Differences from the AMD host code

None beyond the common ones below.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
