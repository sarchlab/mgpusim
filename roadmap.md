# Roadmap: GFX942 (CDNA3) Kernel Emulation

## Goal
Support byte-level correct emulation of a wide range of gfx942 HIP kernels across benchmark suites: SHOC, PolyBench, Rodinia, Parboil, and others. Each benchmark needs:
1. HIP source code compiled to gfx942 HSACO (V5 code object)
2. Go reference implementation for result comparison
3. Byte-level correct emulated results

## Current State (Cycle 1)
- CDNA3 ALU emulator exists (~4000 lines in `amd/emu/cdna3/`)
- V5 HSACO loading works
- VectorAdd is the only benchmark with gfx942 HSACO and passing cdna3 emulation tests
- Existing benchmarks have GCN3 HSACO but NOT gfx942 HSACO
- Several `native/` directories exist but lack compiled `.hsaco` files
- Docker-based compilation workflow established (Makefile pattern in vectoradd/native/)

## Milestones

### M1: Compile existing benchmarks to gfx942 and verify emulation (first batch)
**Budget**: 8 cycles  
**Status**: Not started  
**Scope**: Take the simplest existing benchmarks that already have Go reference implementations, compile their kernels to gfx942 HSACO, and get them passing emulation. Start with: matrixtranspose, floydwarshall, fastwalshtransform, simpleconvolution, fir. These use simpler instruction patterns.

### M2: Compile remaining existing benchmarks to gfx942
**Budget**: 8 cycles  
**Status**: Not started  
**Scope**: atax, bicg, bfs, fft, spmv, stencil2d, aes, kmeans, pagerank, nbody, relu, nw. These may need more ALU instruction support.

### M3: Add Parboil benchmarks
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Identify Parboil benchmarks, convert CUDA→HIP, compile to gfx942, write Go reference, get emulation passing.

### M4: Expand SHOC/PolyBench/Rodinia coverage
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Add benchmarks from these suites that aren't already in the repo.

### M5: Additional benchmark suites and hardening
**Budget**: 10 cycles  
**Status**: Not started  
**Scope**: Find and add more benchmark suites. Stress-test edge cases. Ensure all results are byte-level correct.

## Lessons Learned
(none yet)
