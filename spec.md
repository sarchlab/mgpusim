# Project Specification

## What do you want to build?

Add byte-level correct emulation support for AMD CDNA3 (gfx942) architecture to MGPUSim, enabling benchmarks from multiple suites (SHOC, PolyBench, Rodinia, Parboil, AMD APP SDK, HeteroMark, and others) to run on both GCN3 and CDNA3 architectures.

The system should support:
- V5 HSACO code object format (gfx942)
- Dual-architecture benchmarks (both GCN3 and CDNA3 HSACOs embedded)
- CDNA3-specific instruction set (ALU, memory, control flow)
- Proper kernel argument layout for V5 code objects
- CUDA-to-HIP conversion where needed (manual or via hipify)
- Go reference implementations for all benchmarks to enable result comparison

## How do you consider the project is successful?

The project is successful when:

1. **Broad benchmark coverage**: A wide range of benchmarks from multiple suites pass byte-level verification in CDNA3 mode with `-arch=cdna3 -verify`

2. **No regressions**: All benchmarks continue to work correctly in GCN3 mode — dual-arch support must not break existing functionality

3. **Clean implementation**: Code passes CI checks (lint, tests), follows project conventions, and is maintainable

4. **CI validation**: Milestone acceptance tests are runnable in GitHub Actions CI so progress is continuously verifiable and regressions are caught automatically

5. **Documented patterns**: Each new instruction type, memory addressing mode, or kernarg layout is documented so future benchmarks can follow the pattern

## Constraints

- Must maintain backward compatibility with GCN3
- Must follow existing project architecture and conventions
- Must pass all CI checks (lint, unit tests, integration tests)
- Must use Docker-based ROCm toolchain for HIP compilation
- Must have byte-level correct emulation (not approximate)
- Prefer small, verifiable milestones (2-3 benchmarks) with explicit acceptance commands
- Avoid long local full-suite runs; use focused local checks and CI for broader validation
- Timing simulation is out of scope at this stage

## Resources

- Docker with ROCm 7.1.1 for compiling HIP to gfx942 HSACO
- Existing test infrastructure with 1000+ test cases in GitHub Actions CI
- Existing GCN3 emulator as reference
- AMD ISA documentation for CDNA3
- Existing dual-arch pattern from M1 benchmarks

## Notes

- Focus on incremental progress: add benchmarks in small batches (2-3 at a time)
- When benchmarks fail, investigate root cause before moving on
- Budget time for "unknown unknowns" — new benchmarks often reveal missing instructions or addressing modes
- Don't let technical debt accumulate — clean up branches before merging
