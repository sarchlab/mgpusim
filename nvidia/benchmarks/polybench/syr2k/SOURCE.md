# PolyBench/GPU SYR2K

CUDA host program `syr2k.cu` for the PolyBench/GPU SYR2K benchmark. The host includes the bundled kernel, which the HIP shim makes available to nvcc.

| | |
| --- | --- |
| Kernel source | [`polybench_syr2k.cpp`](../../kernels/polybench/syr2k/native/polybench_syr2k.cpp) |
| Original kernel path | `amd/benchmarks/polybench/syr2k/native/polybench_syr2k.cpp` |
| Kernel snapshot | Commit `31ca066fb44b8a6c5997409936d0e8778a1ef805` |
| Kernel origin | Extracted from `sarchlab/gpu_benchmarks` tier2/polybench_syr2k |
| Kernels | `polybench_syr2k_kernel` |
| Host code follows | `amd/benchmarks/polybench/syr2k/syr2k.go` |
| Host code version | commit `90b1b660` (2026-06-25) on main |
| CUDA port written | 2026-09-23 |
| Binary | `nvidia/benchmarks/bin/polybench-syr2k` |
| Usage | `syr2k [-size N] [-inner-size M]` |

## Differences from the AMD host code

None beyond the common ones below.

Common to all ports: Device buffers that the AMD driver hands out zeroed are cleared with `cudaMemset`; results are checked on the host with the relative-error rule of the Go `Verify` (`|ref - got| / max(|ref|, 1)`), with the reference accumulated in double where that avoids false mismatches.

## Verification status

- Compiles as CUDA (host and device) under clang `-fsyntax-only`.
- Prints `Passed!` when run on a CPU emulation of CUDA with the default arguments.
- Not yet built with nvcc or run on an NVIDIA GPU.
