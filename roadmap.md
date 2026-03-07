# MI300A Timing Simulation — Roadmap

## Goal
Average symmetrical error < 20%, max < 50% across MI300A benchmarks.

## Current Status

### Completed Milestones

**M1** (merged): Basic MI300A timing config
- Frequency 1800MHz, 240 CUs, 32MB L2, SimpleBankedMemory DRAM
- wfPoolSize=8, vgprCount=32768
- L1V: bankLatency=20, MSHR=32
- L2: bankLatency=50, dirLatency=4

**M2** (merged): CU pipeline changes
- VecMem pipeline depth: inst=2, trans=4
- SIMD width confirmed as 16 (reverted incorrect change to 32)

**M3** (merged, PR #27): Correct baseline established
- Reverted SIMD width to 16
- Fixed nbody bug (numBodies calculation)
- Updated compare script to symmetrical error formula
- 33 benchmark data points across 8 benchmarks documented

**M4** (merged, verified): Kernel launch + interconnect fixes
- Cached kernel code object GPU addresses across launches
- Fixed memRangeOverlap adjacency bug (> instead of >=)
- Investigated MMU page-not-found panics (corrupted 64-bit FLAT addresses in timing mode)
- Reduced MI300A switch latency from 140→15 (Infinity Fabric, not PCIe)
- storageAccessor.Write bug fixed

**M5** (merged, verified): s_nop fix + kernel overhead tuning
- Fixed s_nop infinite loop in scheduler.go (default case now advances PC)
- Reduced H2D 14500→500, D2H 8500→300 for MI300A (unified memory)
- Set constantKernelOverhead to 3600 (~2µs GPU-side dispatch)
- Unlocked 4 new benchmarks: bitonicsort, matrixtranspose, fastwalshtransform, simpleconvolution(partial)

**M6** (merged, verified): VCCLO mask + MMU register corruption + FLAT SAddr fix
- Fixed VCCLO write mask inversion (wavefront.go:317)
- Fixed out-of-order memory response handling in computeunit.go
- Fixed FLAT SAddr mode in timing: add scalar base + signed offset
- vectoradd width>4096 now works, atax scaling verified
- M6 benchmark results: 26 data points, mean error ~196%

**M7** (merged, verified): Add gfx942 kernel support + fix emulation bugs
- Added gfx942 kernel support to nbody and matrixmultiplication benchmarks
- Fixed VOP2 SDWA decoder for AES benchmark (SGPR operand handling)
- Fixed VOP3P op_sel/op_sel_hi/neg decoding for packed instructions
- All 21 benchmarks pass CDNA3 emulation with -verify
- Timing benchmark results: 72.4% mean error on 9 data points
- matrixmultiplication shows excellent accuracy (8.5-20.7% at small sizes)
- Cycle estimate: budgeted 6, used 6

**M8** (merged, verified): Remove timing-side scratchpad
- Deleted scratchpadpreparer.go, scratchpad.go from timing
- Rewrote defaultcoalescer.go to read from registers directly via wf.ReadOperand
- Rewrote scalarunit.go SMEM load to use ReadOperand for Base/Offset
- Removed scratchpadPreparer from all CU units (SIMD, Branch, Scalar, LDS, VecMem)
- All 84 CU tests pass; `go build ./...` clean
- vectoradd working range expanded (4032 → all sizes, just slow)
- Cycle estimate: budgeted 8, used 2

### M9: MISSED DEADLINE (budgeted 8, used 8)
- **Achieved**: Merged Harper's FLAT SAddr fix. Collected 65 data points across 10 benchmarks. Avg |error| = 79.6%, median = 31.6%, 66.2% within 50%.
- **Not achieved**: Target was avg < 50%, actual is 79.6%
- **What went wrong**: The team focused on parameter tuning (cache latencies, DRAM width/latency) but missed the fundamental architectural bottleneck: per-CU memory pipeline buffer size limits effective memory bandwidth to ~250 GB/s vs real MI300A's 1+ TB/s. Also introduced SPU=32 which is architecturally incorrect per CDNA3 ISA.

### M9.1: COMPLETE ✅ (budgeted 6, used 6) — PENDING MERGE
- **Branch**: `ares/m9.1-spu16-membuf32` — verified by Apollo, needs merge to main
- SPU=16 (reverted from 32), memPipelineBufferSize=32 (from 8)
- L1V=32KB/5cyc, L2=20cyc, DRAM BPW=4/SL=1, kernel overhead=5400(/2)
- stencil2d default iter=1, fft default passes=1 (matching HW measurement)
- **Results**: 65 data points, avg |error| = 58.2%, median 35.3%, 69.2% within 50%
- Per-kernel: matmul 4.8%, bicg 20.2%, matrixtranspose 34.5%, atax 40.4%, FWT 45.5%, fir 58.1%, stencil2d 61.8%, vectoradd 87.7%, fft 102%, relu 106.8%

## Active Human Issues & Priorities

1. **#344 — Simulation performance too slow**: Create GitHub Actions CI for parallel benchmarks. Simplify sim if needed. Workers should fire-and-check, not block.
2. **#346 — Host OOM**: Never run simulations on host machine. Use GitHub Actions.
3. **#343 — Evidence-based tuning**: Create microbenchmarks. Use documentation citations. Maintain mi300a_calibration.md.
4. **#286 — GPU-side command queueing**: Deferred, revisit when kernel launch overhead is dominant.

## Planned Milestones

### M10: COMPLETE ✅ (budgeted 8, used 2)
- Merged M9.1 to main
- Created GitHub Actions benchmark workflow (.github/workflows/benchmark.yml) with 11 parallel jobs
- Fixed DRAM bandwidth: BPW=1, depth=10, SL=3 → 5.46 TB/s (matches MI300A HBM3 5.3 TB/s)
- Updated mi300a_calibration.md with evidence
- **CI Results (run 22804545319)**: 26/453 matched points, avg error 31.7%, max 80.3%
- Per-kernel: matmul 4.8%, vectoradd 24.5%, relu 27.4%, matrixtranspose 35.3%, FWT 39.5%, stencil2d 59.6%
- **Critical finding**: Most benchmarks (atax, bicg, fft, fir) crash with exit code 2 due to wrong CLI flags in benchmark.yml. nbody crashes with MMU panic.

### M11: COMPLETE ✅ (budgeted 6, used ~2)
- Fixed benchmark.yml CLI flag errors (atax, bicg, fft, fir)
- Added 6 new benchmarks: bitonicsort, floydwarshall, nw, simpleconvolution, pagerank, kmeans
- **CI Run Results (22805338040)**: 90 matched data points (up from 26), avg error 62.2%
- Per-kernel: matmul 4.8%, bicg 16.2%, pagerank 16.9%, vectoradd 24.5%, relu 27.4%, nw 30.1%, bitonicsort 33.8%, matrixtranspose 35.3%, atax 36.5%, FWT 39.5%, floydwarshall 46.7%, fir 59.5%, stencil2d 59.6%, fft 121%, kmeans 379%
- simpleconvolution crashes (exit 1), nbody still panics (MMU bug)
- PR #46 merged

### M12: COMPLETE ✅ (budgeted 6, used ~4)
- Fixed kmeans CI: max-iter 5→1 in benchmark.yml
- Added WithConstantKernelOverhead(1800) to MI300A CP builder (plumbed through CP builder)
- Expanded CI benchmark sizes for vectoradd, stencil2d, fft
- **CI Results (run 22806486504)**: 93 matched data points, avg error 34.5%, median 23.0%
- Per-kernel: matmul 5.2%, relu 9.3%, vectoradd 15.3%, matrixtranspose 15.6%, bicg 21.8%, bitonicsort 24.6%, floydwarshall 27.1%, pagerank 29.2%, nw 35.1%, fir 42.3%, stencil2d 43.1%, atax 45.1%, kmeans 51.2%, FWT 78.8%, fft 110.4%
- PR #47 merged
- **Remaining issues**: stencil2d≥1024 crashes (driver.go:120 slice bounds), nbody panics (MMU), simpleconvolution crashes

### M13: Fix crashes + reduce top-error benchmarks (Budget: TBD — investigating)
**Current focus**: Understanding root causes of top-error benchmarks and crashes before defining fixes.

**Investigation in progress:**
- FFT (110%): Why is sim ~2x too slow? Multi-kernel overhead? 
- FWT (79%): Why is sim too fast at all sizes?
- Driver.go crash: slice bounds at stencil2d ≥1024
- Coverage gaps: bfs, conv2d, im2col, memcopy, spmv, nbody, simpleconvolution

### M14: Final accuracy push (Budget: TBD)
- Target: avg <20%, max <50%

## Lessons Learned
- SIMD=32 was incorrect — always verify against ISA documentation
- Symmetrical error penalizes both over and underestimates more equally
- Small problem sizes are dominated by kernel launch overhead, not compute
- Development must stay in origin repo, not upstream
- Page-not-found crashes caused by corrupted 64-bit FLAT addresses in timing mode
- stencil2d constant timing is correct parallel GPU behavior, not a bug
- atax zero-work was caused by timing-mode SMEM/VCC corruption, not kernel arg layout
- Switch latency needed to be 15 (Infinity Fabric) not 140 (PCIe)
- s_nop infinite loop was root cause for ALL hanging benchmarks
- Kernel launch overhead was modeled wrong (CPU-side H2D delay vs GPU-side scheduler overhead)
- M5 reduced mean error from 341% to 120% — overhead tuning + bug fixes have massive impact
- **CRITICAL**: MMU crashes happen in EMULATION mode too (not just timing) — emulation bugs must be fixed FIRST
- Human's debugging suggestion (test emulation → compare disassembly → compare traces) is the correct systematic approach
- Always test emulation correctness before investigating timing accuracy
- matrixmultiplication accuracy at 8.5-20.7% shows compute pipeline is well-modeled — main issues are in memory subsystem and overhead
- **Scratchpad removal is both cleanup AND bug fix** — the scratchpad was a data-copying indirection layer
- **FLAT SAddr mode detection**: Must use inst.Addr.RegCount (1=SAddr, 2=OFF) not SAddr.IntValue != 0x7F
- The human watches the codebase closely — architectural decisions should be clean and principled
- **M9 lesson**: Pure parameter tuning hits diminishing returns. Root cause analysis BEFORE tuning is essential
- **M9 lesson**: SPU=32 was re-introduced despite being reverted in M2/M3. Must enforce architectural constraints
- **M9 lesson**: The DRAM model (`simplebankedmemory`) is a latency model, not a bandwidth model. The bandwidth bottleneck is in the per-CU memory pipeline (bufferSize=8)
- **M9.1 lesson**: stencil2d and fft defaults (iter=5, passes=2) didn't match real HW measurement methodology. Always verify benchmark settings match the reference data.
- **Operational lesson**: Human explicitly demands we stop running simulations on the host (OOM, issue #346) and use GitHub Actions instead. Must create CI workflows for benchmark evaluation.
- **Operational lesson**: Parameter tuning must be evidence-based (issue #343). Create microbenchmarks, cite documentation, document decisions in mi300a_calibration.md.
- **Cycle estimates**: M1-M4 ~20 cycles; M5 ~5; M6 ~8; M7 ~6; M8 ~2; M9 ~8 (failed); M9.1 ~6; M10 ~2
- **M10 lesson**: Always verify CI workflow output. The workflow was created but 4/11 benchmarks had wrong CLI flags (atax used -row/-col instead of -x/-y, fft used -length instead of -MB, fir used nonexistent -taps flag). This wasted the entire CI run for those benchmarks.
- **M10 lesson**: Coverage matters more than accuracy at this stage. With only 26/453 matched points (5.7%), we don't have enough data to make good accuracy decisions. Expanding coverage first gives us a true picture.
- **M11 lesson**: Coverage expansion (26→90 points) revealed that kmeans (379%) and fft (121%) are massive outliers dragging the average from 32.2% to 62.2%. Without these two benchmarks, the simulator is already decent. Root cause analysis of outliers is more impactful than broad parameter tuning.
- **M11 lesson**: Multi-kernel-launch benchmarks may accumulate kernel launch overhead that dwarfs the actual compute time, especially at small problem sizes. Need to understand how many launches each benchmark does.
