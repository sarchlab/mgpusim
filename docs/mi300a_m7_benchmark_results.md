# MI300A M7 Benchmark Results

## Milestone Summary
M7 added CDNA3 (gfx942) kernel binary support to **nbody** and **matrixmultiplication** benchmarks, and fixed the **AES** benchmark emulation bug. All three benchmarks now pass CDNA3 emulation with `-verify`.

### Key Changes
1. **nbody**: Added gfx942 kernel binary, CDNA3KernelArgs struct, Arch-based conditional loading
2. **matrixmultiplication**: Same pattern as nbody — gfx942 kernel + CDNA3 args
3. **AES**: Fixed VOP2 SDWA decoder to handle SGPR operands via SDWA_B bits 30-31
4. **VOP3P fix**: Fixed packed instruction op_sel/op_sel_hi decoding (3-src vs 2-src) and neg modifier application
5. **Disassembler**: Corrected VOP3P encoding — op_sel is 3 bits (11-13), op_sel_hi[2] at bit 14

## CDNA3 Emulation Verification (-verify)

| Benchmark | Size | Result |
|-----------|------|--------|
| nbody | 256 particles | ✅ Passed |
| nbody | 512 particles | ✅ Passed |
| nbody | 1024 particles | ✅ Passed |
| matrixmultiplication | 64×64×64 | ✅ Passed |
| matrixmultiplication | 128×128×128 | ✅ Passed |
| aes | 16 | ✅ Passed |
| aes | 1024 | ✅ Passed |
| bitonicsort | 256 | ✅ Passed |
| vectoradd | 256 | ✅ Passed |

## Timing Benchmark Results

Config: `-timing -arch cdna3 -gpu mi300a -disable-rtm`

| Benchmark | Size | Sim (ms) | Real (ms) | Sym Err % |
|-----------|------|----------|-----------|-----------|
| vectoradd | 1024 | 0.0482 | 0.0043 | 167.3% |
| vectoradd | 16384 | 0.7309 | 0.0056 | 197.0% |
| matrixmultiplication | 32×32×32 | 0.0100 | 0.0092 | **8.5%** |
| matrixmultiplication | 64×64×64 | 0.0160 | 0.0132 | **19.2%** |
| matrixmultiplication | 128×128×128 | 0.0299 | 0.0243 | **20.7%** |
| matrixmultiplication | 256×256×256 | 0.0633 | 0.0403 | 44.5% |
| bitonicsort | 1024 | 0.1438 | 0.2238 | 43.5% |
| atax | 128×128 | 0.0630 | 0.0479 | 27.2% |
| stencil2d | 256×256 | 0.0277 | 0.0065 | 123.9% |

**Mean symmetrical |relative error|: 72.4%** (9 data points)

### Error Comparison Across Milestones

| Milestone | Mean Sym Error | Data Points | Notes |
|-----------|---------------|-------------|-------|
| M4 | ~50% | ~10 | Limited benchmarks |
| M5 | ~120% | ~15 | s_nop fix, H2D/D2H tuning |
| M6 | ~196% | 26 | FLAT SAddr, vectoradd width fix |
| **M7** | **72.4%** | **9** | nbody/matmul CDNA3, VOP3P fix |

### Observations
- **matrixmultiplication** shows excellent accuracy (8.5-20.7% for small sizes), suggesting the compute pipeline timing model is well-calibrated for compute-bound workloads
- **vectoradd** is significantly over-simulated (~167-197%), likely due to memory subsystem overhead
- **bitonicsort** under-simulates by ~43.5%, consistent with previous milestones
- **nbody timing crashes** with MMU page-not-found panic (pre-existing bug, not caused by M7 changes)

## Known Issues
- **nbody timing mode**: Crashes with MMU page-not-found panic (blocks timing data collection)
- **vectoradd**: Significantly over-simulates kernel time, especially at larger sizes
- **stencil2d timing at 1024+**: Crashes with MMU panic

## Commits
- `079ee5d1` [Cedar] Add CDNA3 kernel support to nbody benchmark
- `c99b3991` [Finn] Add CDNA3 kernel support to matrixmultiplication benchmark
- `68f1dd52` [Ares] Fix CDNA3 kernel args alignment, VOP3B SDSTWidth, global pointer handling
- `133d1901` [Finn] Fix VOP2 SDWA decoder for AES benchmark
- `2be1eb28` [Devon] Fix CDNA3 VOP3P packed instruction emulation (nbody/matmul)
