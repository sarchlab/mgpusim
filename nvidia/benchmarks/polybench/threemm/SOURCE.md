# PolyBench/GPU 3MM

CUDA host program `threemm.cu` for the PolyBench/GPU 3MM benchmark. The GPU kernels are not copied: `threemm.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/polybench/threemm/native/polybench_3mm.cpp` |
| Kernel version | commit `146a4c8c` (2026-06-17) on main |
| Kernel origin | Extracted from `sarchlab/gpu_benchmarks` tier2/polybench_3mm |
| Kernels | `mm3_kernel1`, `mm3_kernel2`, `mm3_kernel3` |
| Host code follows | `amd/benchmarks/polybench/threemm/threemm.go` |
| Host code version | commit `90b1b660` (2026-06-25) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/threemm` |
| Usage | `threemm [-size N]` |

## Differences from the AMD host code

- One `-size` flag sets NI = NJ = NK = NL = NM, as the sample does.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
