# PolyBench/GPU 2MM

CUDA host program `twomm.cu` for the PolyBench/GPU 2MM benchmark. The GPU kernels are not copied: `twomm.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/polybench/twomm/native/polybench_2mm.cpp` |
| Kernel version | commit `146a4c8c` (2026-06-17) on main |
| Kernel origin | Extracted from `sarchlab/gpu_benchmarks` tier2/polybench_2mm |
| Kernels | `mm_kernel` |
| Host code follows | `amd/benchmarks/polybench/twomm/twomm.go` |
| Host code version | commit `90b1b660` (2026-06-25) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/twomm` |
| Usage | `twomm [-size N]` |

## Differences from the AMD host code

- Uses the 1e-2 tolerance of `twomm.go` because the error of D carries into E.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
