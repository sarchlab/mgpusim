# Roadmap: GFX942 (CDNA3) Kernel Emulation

## Goal
Support byte-level correct emulation of a wide range of gfx942 HIP kernels across benchmark suites: SHOC, PolyBench, Rodinia, Parboil, and others. Each benchmark needs:
1. HIP source code compiled to gfx942 HSACO (V5 code object)
2. Go reference implementation for result comparison
3. Byte-level correct emulated results
4. Acceptance tests runnable in GitHub Actions CI

## Current State (After M2 deadline miss)
- CDNA3 ALU emulator exists (~4000 lines in `amd/emu/cdna3/`)
- V5 HSACO loading works
- 7 benchmarks pass with `-arch=cdna3 -verify` on `main`: vectoradd, memcopy, matrixtranspose, floydwarshall, fastwalshtransform, fir, simpleconvolution
- M2 integration work exists on branch `ares/cdna3-benchmarks-m2` but has not been merged
- On `main`, all 5 M2 sample binaries build and pass GCN3, but all currently fail CDNA3:
  - atax: SOP2 opcode 11 not implemented
  - bitonicsort: SOP2 opcode 34 not implemented
  - matrixmultiplication: SOP1 opcode 48 not implemented
  - kmeans: SOP2 opcode 11 not implemented
  - bicg: page-table fault

## Milestones

### M1: Compile existing benchmarks to gfx942 and verify emulation (first batch)
**Budget**: 8 cycles  
**Status**: ✅ COMPLETE (cycle 2)  
**Scope**: matrixtranspose, floydwarshall, fastwalshtransform, simpleconvolution, fir — all pass `-arch=cdna3 -verify` and maintain GCN3 compatibility.

### M2 (parent): Add gfx942 support to second batch of existing benchmarks
**Budget**: 8 cycles  
**Status**: ❌ MISSED DEADLINE (8/8 cycles used)  
**Original Scope**: atax, bicg, bitonicsort, matrixmultiplication, kmeans.

### M2.1: Resolve CDNA3 opcode gaps for 4 M2 benchmarks
**Budget**: 4 cycles  
**Status**: Planned  
**Scope**:
- Make `atax`, `bitonicsort`, `matrixmultiplication`, and `kmeans` pass `-arch=cdna3 -verify`
- Preserve GCN3 behavior for these 4 benchmarks
- Preserve previously passing M1 benchmark behavior on both arches

### M2.2: Fix bicg page-table fault and add CI acceptance tests
**Budget**: 4 cycles  
**Status**: Planned  
**Scope**:
- Make `bicg` pass `-arch=cdna3 -verify`
- Add a focused GitHub Actions acceptance workflow for M2 benchmarks in both GCN3 and CDNA3 modes
- Ensure workflow is used for milestone acceptance and regression checking

### M3: Add gfx942 support to remaining existing benchmarks
**Budget**: 8 cycles  
**Status**: Not started  
**Scope**: bfs, fft, spmv, stencil2d, aes, pagerank, nbody, relu, nw.

### M4: Add Parboil benchmarks (CUDA→HIP conversion)
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Identify Parboil benchmarks, convert CUDA→HIP, compile to gfx942, write Go reference, get emulation passing.

### M5: Expand SHOC/PolyBench/Rodinia/additional coverage
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Add benchmarks from these suites not already covered; find and integrate additional benchmark suites.

## Lessons Learned
- **Deadlines need finer slicing**: A 5-benchmark mixed milestone was too large; split by failure type (opcode gaps vs memory faults).
- **Dual-arch pattern is required**: Always embed both GCN3 and gfx942 HSACO and load by arch.
- **KernelArgs layout must match HSACO metadata exactly**: Hidden argument/padding mismatch causes hard-to-debug runtime errors.
- **CDNA3 bring-up is iterative**: New benchmark batches reveal missing instructions and memory-model corner cases.
- **CI must validate milestone acceptance**: Relying only on local spot checks allows late regressions and misses human-requested acceptance automation.
