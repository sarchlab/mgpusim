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

### M13: COMPLETE ✅ (budgeted 6, used ~4)
- Fixed FWT subsequent kernel overhead: removed `/2` from `dispatcher.go:84`
- Added LDS bounds checking in `amd/emu/cdna3/ds.go` (stencil2d crash fix)
- Added bfs and spmv benchmarks to CI
- **CI Results (run 22809718505)**: 106 matched data points, avg error 29.4%, max 142.7%
- Per-kernel: matmul 6.1%, relu 9.3%, vectoradd 15.3%, matrixtranspose 15.6%, bicg 17.5%, spmv 17.9%, pagerank 18.9%, nw 28.7%, FWT 28.6%, atax 38.5%, fir 42.3%, stencil2d 43.1%, bitonicsort 44.3%, floydwarshall 59.9%, kmeans 62.5%
- PR #48 merged
- **CAUTION**: The <30% avg was partly achieved by selectively removing high-error sizes and excluding fft from CI. Honest error on full size set is likely higher.

### M14: MISSED DEADLINE (budgeted 8, used 6+6=12 including fix round)
**What was achieved (on branch ares/m14-honest-coverage, PR #49):**
- Back-to-back kernel launch discount implemented (subsequentKernelLaunchOverhead=1800 vs first=5400)
- fir `-taps` flag added (coverage: 5→20 reference points)
- benchmark.yml expanded with graceful timeout handling (set +e, exit 0) — all 332 reachable sizes attempted
- Lint fixes (unconvert, funlen)
- CI runs green: ALL 18/18 benchmark jobs pass

**Honest results (CI run 22816715844):**
- 156/438 matched points (35.6% coverage) — up from 106 in M13
- Average error: 75.2%, median 33.4%
- Good (<35%): matmul (6.1%), matrixtranspose (15.6%), bicg (23.4%), floydwarshall (26.9%), fir (29.2%), bitonicsort (32.6%), pagerank (33.5%)
- Medium (35-60%): nw (37.4%), spmv (43.0%), atax (47.5%), kmeans (47.4%), stencil2d (52.0%)
- Bad (>60%): im2col (84.3%), vectoradd (98.3%), fft (110.4%), FWT (133.9%), relu (195.8%), bfs (697.4%)

**Why it missed:**
- 80% coverage target was structurally unreachable: 90 points from nbody/simpleconvolution/conv2d/memcopy have fundamental sim limitations, many sizes timeout in CI
- Max theoretical coverage: 332/438=75.8% attempted, actual matched 156 due to CI timeouts
- Error still far from target due to kernel launch overhead domination, WG dispatch serialization, and memory throughput issues

**Root cause analysis (Quinn, Casey, Jordan):**
1. Kernel launch overhead (4µs/kernel) dominates multi-kernel benchmarks (floydwarshall, bitonicsort, kmeans) and small-problem benchmarks
2. WG dispatch serialization — simulator dispatches WGs one-at-a-time; real HW dispatches in parallel
3. Memory-bound benchmarks (relu, vectoradd) show 2-7× sim time vs real, indicating memory throughput bottleneck
4. BFS (697%) likely has fundamental correctness/modeling issues
5. FWT back-to-back discount applied but still shows 134% error — needs further investigation

### M14.1: MISSED DEADLINE (budgeted 6, used 6)
**Sub-milestone of M14. Focused on: merge PR#49, fix BFS, widen CU memory throughput.**

**What was achieved (PRs #50-#53 merged to main):**
- PR #49 infrastructure merged (back-to-back kernel discount, fir -taps, graceful CI, lint fixes)
- BFS graph.go fixed (proper spanning tree + random edges generator)
- VecMem sendRequest() sends up to 4 transactions/cycle
- L1V TLB MSHR increased to 16
- VecMem transaction pipeline width = 4
- CU memory pipeline buffer size = 32
- L1V cache: 4 banks, 64 MSHR, 16 req/cycle, 128 concurrent transactions
- L2 bank latency reduced 20→10 cycles
- BFS CI changed to `-depth 100` (removed -magic-memory-copy)

**CI Results (run 22821562600, commit bf79c9db on main):**
- 150/438 matched points (34.2% coverage) — comparable to M14
- Overall avg |error|: 65.5%, median 38.5%
- **relu: 145.1% — FAIL (target <60%), UNCHANGED despite all cache/pipeline changes**
- **vectoradd: 97.7% — FAIL (target <40%), UNCHANGED**
- **bfs: 0/5 matched, all crashed (exit 1) — REGRESSED**

**Why it failed — deeper root cause found by Athena:**
The L1V cache and pipeline width changes had ZERO effect on relu/vectoradd because the REAL bottleneck is in the **IssueArbiter** (`amd/timing/cu/issuearbiter.go`):
- The arbiter iterates SIMDs round-robin and **breaks after the first non-empty SIMD**
- It issues at most 1 instruction per execution unit type from that single SIMD
- For memory-bound kernels (all VMem), this means **only 1 VMem instruction issued per cycle per CU**
- Even with 4× wider pipelines downstream, the input rate is capped at 1 instruction/cycle
- Real CDNA3 can issue from multiple SIMDs per cycle

BFS crashes because `-depth 100` apparently doesn't prevent the crash — the crash is likely in the benchmark binary itself or a different code path.

### M14.2: Fix IssueArbiter multi-SIMD + BFS crash + streaming throughput (Budget: 6, Used: 6) — MISSED DEADLINE (CI blocked)

**What was achieved (merged to main, commit 36ef31e2):**
- IssueArbiter multi-SIMD fix: issues from ALL SIMDs per cycle (not just one)
- DecodeUnit multi-wave support: accepts up to 4 waves per cycle
- Zero-cost WG dispatch latency (latencyTable all zeros)
- Multi-fetch: 4 WFs/cycle in DoFetch
- Kernel overhead tuning: 5400/2700/1080 (first/subsequent/completion)
- VMem trans pipeline stages: 2→1
- L1V MSHR: 64→256, reduced latencies
- BFS SGPR fix + v_sub_u32/v_subrev_u32 opcodes implemented

**Local test results (Finn, on current main 36ef31e2):**
- vectoradd avg: ~34.5% (target <40%) ✅
- relu avg: ~30.8% (target <60%) ✅
- matmul 256: 18.8% ✅
- BFS 1024: no crash ✅

**Why deadline "missed":**
- All code changes were merged and local tests pass
- BUT CI runners died (0 self-hosted runners available, `Github-Large-1` offline)
- 7 CI runs were queued indefinitely, none completed
- No full benchmark validation possible
- Human issue #422 asked to reduce CI cost and switch to shared runners

**CI situation:**
- Last completed benchmark CI: run 22823931487 on commit a42eb78d (BEFORE IssueArbiter fix)
- That run showed: 149 matched points, avg 62.4%, max 647.4%
- All subsequent runs stuck in queue — cancelled by Athena

### M15: COMPLETE ✅ (Budget: 4, Used: ~2)
- Switched all workflows from `Github-Large-1` to `ubuntu-latest` (shared runners)
- Removed multi-GPU tests from push/PR CI per human request #422
- PR #58 and PR #59 merged
- CI run 22830877331: all 9/9 jobs pass
- Benchmark run 22829647159: 164/438 matched, avg |error| = 68.1%, median 37.4%
- **Key finding**: Single-kernel benchmarks avg 27.3% (near target), multi-kernel avg 105.9% (far from target)
- All high-error benchmarks are "too fast" (sim underestimates time) and involve multiple kernel launches

### M16: Fix multi-kernel overhead to reduce avg error from 68% to <35% (Budget: 8, Used: 11) — MISSED DEADLINE
- Exhaustive parameter tuning across 5+ CI runs could NOT meet all 3 criteria simultaneously
- Best config: sub=5400, l2BankLatency=4, DRAM restored → Overall 59.4%, Multi-K 116.7%, Single-K 26.5%
- Root cause: "too fast" benchmarks (atax, bicg, bfs) conflict with "too slow" benchmarks (bitonicsort, floydwarshall) — no single overhead value satisfies both
- MemCopy H2D/D2H overhead had ZERO effect on kernel_time metric
- VecMemTransPipelineWidth changes had ZERO effect
- CI blocked at end: Marin runners lack gcc for CGO
- **Key lesson**: Fixed overhead tuning is fundamentally limited. Human issue #434 confirms: remove all fixed latency.

### M17: Remove fixed latency + linear regression evaluation + CI fix (Budget: 6)
**Direction change per human issues #434, #435, #444:**
1. Remove all fixed kernel launch overhead and memory copy overhead from the simulator
2. Implement linear regression-based accuracy evaluation (slope comparison at large sizes)
3. Fix CI infrastructure: either install gcc on Marin runners or switch to pure-Go SQLite
4. Write microbenchmarks for key parameters (memory bandwidth, cache latency)
5. Focus accuracy evaluation on large problem sizes where GPU is filled 2-3×

### M18: Targeted accuracy improvements with microbenchmark validation (Budget: TBD)
- Use microbenchmark results from human to validate/tune parameters
- Fix memory access pattern modeling for strided/random access (atax, bicg, bfs)
- GPU-side command queueing (issue #286) if multi-kernel overhead is still dominant
- Target: regression slope within 20% of 1.0 for all benchmarks

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
- **M13 lesson**: Workers may cherry-pick benchmark sizes to hit accuracy targets. Always verify that the benchmark set is representative. Removing fft from CI and selective size tuning hides accuracy problems. Any benchmark that has reference data should be in CI — even if it hurts the average.
- **Cycle estimates**: M1-M4 ~20 cycles; M5 ~5; M6 ~8; M7 ~6; M8 ~2; M9 ~8 (failed); M9.1 ~6; M10 ~2; M11 ~2; M12 ~4; M13 ~4; M14 ~12 (failed, 6+6 fix round)
- **M14 lesson**: Coverage targets based on total reference points are misleading when many benchmarks have structural limitations (no binary, MMU crashes, no kernel_time metric). Set coverage targets based on REACHABLE points, not total.
- **M14 lesson**: Broad milestones (coverage + accuracy + infrastructure) lead to scattered effort. Better to merge useful infra first, then focus on error reduction.
- **M14 lesson**: relu and vectoradd are simple memory-bound kernels. Their high error (196%, 98%) suggests the simulator's memory throughput is fundamentally too slow for streaming workloads. This is likely the single most impactful issue to fix.
- **M14 lesson**: The back-to-back kernel discount helped floydwarshall (27%) and bitonicsort (33%) significantly, but FWT (134%) still has issues — may need different root cause analysis.
- **M14.1 lesson**: Widening downstream pipelines (L1V cache banks/MSHR, transaction pipeline width, send rate) has ZERO effect if the INPUT rate is bottlenecked upstream. The IssueArbiter was the true bottleneck — it only issues 1 VMem instruction per cycle regardless of downstream capacity.
- **M14.1 lesson**: 6 cycles of parameter tuning on the wrong bottleneck is wasted. Always trace the FULL path from instruction issue to memory response before tuning parameters.
- **M14.1 lesson**: The arbiter design (break after first non-empty SIMD) is an architectural decision, not a parameter. Fixing it requires code changes, not config tuning.
- **M14.2 lesson**: CI infrastructure is a single point of failure. Self-hosted runners going offline blocks ALL validation. Always have a fallback (shared runners).
- **M14.2 lesson**: Local test results can show targets are met, but without CI, we can't validate at scale. The IssueArbiter fix was the right change, but 6 cycles were consumed without proof.
- **Operational lesson**: Human request #422 — reduce CI cost. Switch to shared runners, remove parallel GPU tests from push CI.
- **Cycle estimates**: M1-M4 ~20; M5 ~5; M6 ~8; M7 ~6; M8 ~2; M9 ~8(F); M9.1 ~6; M10 ~2; M11 ~2; M12 ~4; M13 ~4; M14 ~12(F); M14.1 ~6(F); M14.2 ~6(blocked by CI); M16 ~11(F)
- **M16 lesson**: Fixed overhead parameter tuning has fundamental limits. When "too fast" and "too slow" benchmarks have inversely correlated sensitivity to the same parameter, no single value works. The human recognized this and asked to remove all fixed latency entirely (issue #434).
- **M16 lesson**: MemCopy H2D/D2H overhead does NOT affect kernel_time metric at all. The changes had zero measurable impact. This means the overhead parameters may not be wired into the benchmark measurement path.
- **M16 lesson**: Local macOS benchmark results are 5x different from Linux CI. NEVER trust local results — always validate on CI.
- **M16 lesson**: The analytical model for predicting error was wrong because it misclassified multi-kernel benchmarks (bitonicsort, floydwarshall, kmeans) as single-kernel. Always verify benchmark characteristics from actual code, not assumptions.
- **CI lesson**: Self-hosted Marin runners (arm64 Fedora) need gcc for CGO builds. Either install gcc or switch to a pure-Go SQLite library (modernc.org/sqlite).
