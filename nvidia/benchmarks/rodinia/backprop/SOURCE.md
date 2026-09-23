# Rodinia Backprop

CUDA host program `backprop.cu` for the Rodinia Backprop benchmark. The GPU kernels are not copied: `backprop.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/rodinia/backprop/native/rodinia_backprop.cpp` |
| Kernel version | commit `7155c64f` (2026-07-04) on main |
| Kernel origin | Extracted from `sarchlab/gpu_benchmarks` tier2/rodinia_backprop |
| Kernels | `forward_hidden`, `forward_output`, `backward_output_delta`, `backward_hidden_delta`, `update_w1`, `update_w2` |
| Host code follows | `amd/benchmarks/rodinia/backprop/backprop.go` |
| Host code version | commit `7155c64f` (2026-07-04) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/backprop` |
| Usage | `backprop [-input I] [-hidden H] [-output O]` |

## Differences from the AMD host code

- Uses the 1e-3 denominator floor of `backprop.go` because the weights are small.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
