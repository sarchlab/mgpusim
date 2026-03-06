# Roadmap: MI300A Timing Simulation Accuracy

## Goal
Achieve <20% average error and <50% max error for MI300A timing simulation vs real hardware measurements (120 CU config).

## Status
- M1.1 COMPLETE and VERIFIED — DRAM controller, WfPoolSize, VGPR fixes applied
- PR #251 open on sarchlab/mgpusim (not merged)
- Need to establish baseline accuracy with current fixes before next round of tuning
- Human requested DRAM model evaluation (issue #234) — in progress

## Completed Milestones

### M1: Baseline Infrastructure (cycles: 2) — FAILED
Too broad for execution team. Lesson: give concrete code changes, not research tasks.

### M1.1: Fix MI300A Timing Parameters (cycles: 4) — COMPLETE ✅
- Switched DRAM from idealmemcontroller to HBM timing model
- Fixed WfPoolSize from 10 to 8 per SIMD (MI300A-specific)
- Fixed VGPR count from 16384 to 32768 per SIMD (MI300A-specific)
- Benchmark scripts cherry-picked (compare_sim_vs_real.py, run_sim_benchmarks.sh)
- PR #251 created on sarchlab/mgpusim
- Initial results: stencil2d improved ~20%, others ~1-2% (tiny problem sizes only)

## Current Work (Planning Phase)

### Gathering Data
- Alex: Evaluating Akita DRAM model for HBM3 accuracy (issue #238)
- Blake: Running comprehensive benchmark accuracy comparison (issue #239)

### Key Questions to Answer
1. What is our actual accuracy now? (Only tested at tiny sizes so far)
2. What are the maximum problem sizes the simulator can handle per benchmark?
3. What does the Akita DRAM model get wrong for HBM3?
4. Which parameters have the biggest impact on remaining error?

## Upcoming Milestones

### M1.2: Measure Accuracy & Tune Further (cycles: 4) — PENDING
- Run full benchmark suite (small-medium problem sizes) to measure accuracy
- Tune remaining parameters: L2 cache size (8MB→24MB+), clock frequency (1500→1900MHz), L1V latency, SIMD latency
- Target: <30% average error
- Dependent on accuracy baseline from Blake's analysis

### M1.3: Fine-Tuning & Final Accuracy (cycles: 4) — PENDING
- Address individual benchmark outliers
- Tune TLB, MSHR, memory interleaving parameters
- Target: <20% average, <50% max

### M-DRAM: DRAM Model Evaluation Report (cycles: 2) — PENDING
- Separate deliverable for human issue #234
- Create test programs to evaluate HBM model
- Document what Akita team needs to change
- Create GitHub issue with findings for human

## Lessons Learned
- **M1 failure**: Don't give research tasks to execution teams. Do analysis in planning phase, give concrete code changes.
- **Tiny benchmarks mislead**: Testing at 64-element sizes doesn't reveal real accuracy. Need larger problem sizes.
- **Parameter fixes are MI300A-specific**: Good design — don't change defaults for other GPU configs.
- **DRAM model is a separate concern**: Akita's DRAM model accuracy is a dependency we can't fully fix in mgpusim alone.
