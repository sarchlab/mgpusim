# Rodinia Needleman-Wunsch

CUDA host program `nw.cu` for the Rodinia Needleman-Wunsch benchmark. The GPU kernels are not copied: `nw.cu` `#include`s the HIP kernel file of the AMD benchmark, which nvcc compiles unchanged through `../../include/hip/hip_runtime.h`.

| | |
| --- | --- |
| Kernel source | `amd/benchmarks/rodinia/nw/native/nw.cpp` |
| Kernel version | commit `2d04fc56` (2026-03-05) on main |
| Kernel origin | HIP translation of `nw.cl` (Rodinia OpenCL) in the same directory |
| Kernels | `nw_kernel1`, `nw_kernel2` |
| Host code follows | `amd/benchmarks/rodinia/nw/benchmark.go` |
| Host code version | commit `7d8578f7` (2026-06-15) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/nw` |
| Usage | `nw [-length L] [-penalty P]   (L must be a multiple of 64)` |

## Differences from the AMD host code

- `nw_kernel2` is launched with `blk` from `block_width - 1` down to 1, as in the original Rodinia code. `benchmark.go` counts upward, which gives wrong results from length 256 on (confirmed with the CPU emulator: 12288 mismatches at 256).
- The sequences come from a fixed-seed LCG; `benchmark.go` uses Go's `math/rand`.
- The BLOSUM62 table is copied from `benchmark.go`, including its row 12 that has 23 entries (the missing entry is never read because sequence values are 1..10).
- Adds a `-penalty` flag (default 10, as in `benchmark.go`).

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
