# Roadmap: GFX942 (CDNA3) Kernel Emulation

## Goal
Support byte-level correct emulation of a wide range of gfx942 HIP kernels across benchmark suites: SHOC, PolyBench, Rodinia, Parboil, and others. Each benchmark needs:
1. HIP source code compiled to gfx942 HSACO (V5 code object)
2. Go reference implementation for result comparison
3. Byte-level correct emulated results

## Current State (After M1)
- CDNA3 ALU emulator exists (~4000 lines in `amd/emu/cdna3/`)
- V5 HSACO loading works
- 7 benchmarks now pass with `-arch=cdna3 -verify`: vectoradd, memcopy, matrixtranspose, floydwarshall, fastwalshtransform, fir, simpleconvolution
- Dual-arch pattern established: each benchmark embeds both GCN3 and gfx942 HSACOs, loads conditionally based on `-arch` flag
- Docker-based HIP compilation workflow established (Makefile pattern)
- KernelArgs hidden args pattern established for V5 code objects

## Milestones

### M1: Compile existing benchmarks to gfx942 and verify emulation (first batch)
**Budget**: 8 cycles  
**Status**: ✅ COMPLETE (cycle 2)  
**Scope**: matrixtranspose, floydwarshall, fastwalshtransform, simpleconvolution, fir — all pass `-arch=cdna3 -verify` and also maintain GCN3 compatibility.

### M2: Add gfx942 support to second batch of existing benchmarks
**Budget**: 8 cycles  
**Status**: Not started  
**Scope**: atax, bicg, bitonicsort, matrixmultiplication, kmeans — these have OpenCL kernels and existing Go reference implementations. They need:
1. HIP source files + Makefiles, compiled to gfx942 HSACO
2. Dual-arch Go integration following M1 pattern
3. Any missing CDNA3 instructions fixed

### M3: Add gfx942 support to remaining existing benchmarks
**Budget**: 8 cycles  
**Status**: Not started  
**Scope**: bfs, fft, spmv, stencil2d, aes, pagerank, nbody, relu, nw. These may need more ALU instruction support or address bug fixes (page faults).

### M4: Add Parboil benchmarks (CUDA→HIP conversion)
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Identify Parboil benchmarks, convert CUDA→HIP, compile to gfx942, write Go reference, get emulation passing.

### M5: Expand SHOC/PolyBench/Rodinia/additional coverage
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Add benchmarks from these suites that aren't already in the repo. Find and add more benchmark suites.

## Lessons Learned
- **Dual-arch pattern**: embed both GCN3 and gfx942 HSACOs, load conditionally based on Arch field. Move loadProgram() to Run() so Arch is set before loading.
- **KernelArgs layout**: gfx942 V5 code objects use hidden kernel args (8 bytes each for X/Y/Z offsets). The struct must match exactly — use disassembly to verify argument offsets.
- **V5 code objects**: Different kernel descriptor layout than V4. Work-item IDs are packed in v0 for gfx942.
- **Opcode shifts**: GCN3 and gfx942 have different opcode numbers for the same instructions. Never run GCN3 HSACO through CDNA3 emulator.
- **File naming on macOS**: macOS is case-insensitive, so HIP source files can't have same basename as existing OpenCL files (even different case). Use `_hip` suffix to avoid collisions.
- **extern "C" kernels**: Use `extern "C" __global__` to prevent C++ name mangling, matching kernel names expected by Go code.
