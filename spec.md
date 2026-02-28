# Project Specification

## What do you want to build?

We need to support emulating a wide range of gfx942 hip kernels. This work is already partially started. Please complete the task.

## How do you consider the project is success?

We should be able to support all the benchmarks from SHOC, PolyBench, Rodinia, and Parboil. We should also find and integrate additional benchmark suites. CUDA benchmarks can be converted to HIP either manually or with hipify. For every benchmark, always provide a Go reference implementation so calculation results can be compared.

Use the existing Docker-based workflow to compile kernels. The core goal is byte-level correct kernel emulation results. Timing simulation is out of scope at this stage.

In addition, milestone acceptance tests must be runnable in GitHub Actions CI so progress is continuously verifiable and regressions are caught automatically.

## Constraints

- Preserve existing GCN3 behavior while adding gfx942/CDNA3 support.
- Keep benchmark integration dual-arch (GCN3 + CDNA3) where applicable.
- Prefer small, verifiable milestones with explicit acceptance commands.
- Avoid long local full-suite runs; use focused local checks and CI for broader validation.
