# Roadmap: MI300A Timing Simulation Accuracy

## Goal
Achieve <20% average error and <50% max error for MI300A timing simulation vs real hardware measurements (120 CU config).

## Status
- Research phase complete (Alex, Blake, Casey workers gathered hardware specs, identified discrepancies, built scripts, collected initial sim data)
- M1 (original broad baseline milestone) missed deadline — was too vague for execution team
- Breaking down into specific, concrete milestones

## Completed Research Findings
- **Ideal memory controller** is the #1 accuracy problem (flat 100-cycle latency, no bandwidth modeling)
- **Wavefront pool size** wrong (10 vs 8 per SIMD)
- **VGPR count** wrong (16384 vs 32768 per SIMD)
- **SIMD execution latency** potentially too slow
- Initial sim data collected for 5 benchmarks at small sizes (20 data points)
- Comparison script and runner script created on branches

## Milestones

### M1: Baseline Infrastructure (cycles: 2) — DEADLINE MISSED
**Status: FAILED** — Too broad, team had no clear concrete tasks
**Lesson: Need very specific code changes, not research tasks, for execution team**

### M1.1: Fix MI300A Timing Parameters (cycles: 4)
**Status: PENDING**
Concrete code changes to `amd/samples/runner/timingconfig/mi300a/builder.go` and `amd/timing/cu/`:
1. Switch from `idealmemcontroller` to the existing `createDramControllerBuilder()` DRAM model in `buildDRAMControllers()`
2. Fix wavefront pool size from 10 to 8 per SIMD in CU builder
3. Fix VGPR count from 16384 to 32768 per SIMD in CU builder
4. Merge benchmark scripts (compare_sim_vs_real.py, run_sim_benchmarks.sh) from research branches to main
5. Run a subset of benchmarks to measure before/after accuracy
6. Create PR on upstream with all changes

### M1.2: Measure Accuracy & Tune Further (cycles: 4)
**Status: PENDING**
- Run full benchmark suite (small problem sizes) to measure accuracy
- Tune remaining parameters: L2 cache size, clock frequency, L1V latency, SIMD latency
- Iterate based on per-benchmark error analysis
- Target: <30% average error

### M1.3: Fine-Tuning & Final Accuracy (cycles: 4)
**Status: PENDING**
- Address individual benchmark outliers
- Tune TLB, MSHR, memory interleaving parameters
- Cross-validate with 120 CU vs 240 CU data
- Target: <20% average, <50% max

## Lessons Learned
- **M1 failure**: The original M1 was a research/analysis milestone given to an execution team. Ares's team needs concrete code changes with specific files and line numbers, not open-ended investigation.
- **Break down further**: When milestones involve "analyze and decide", do the analysis in the planning phase (Athena's workers), then give Ares specific implementation tasks.
- **Scripts exist**: Blake and Casey already created the needed infrastructure scripts. Next milestone should build on that work.
