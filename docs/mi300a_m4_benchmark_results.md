# MI300A M4 Benchmark Results — After Interconnect Latency Fix

## Overview

Results after merging the MI300A interconnect latency fix (`ares/m4-stencil-fix`), which:
1. **Reduces PCIe switch latency from 140 to 15 cycles** for MI300A (uses on-die Infinity Fabric, not PCIe)
2. **Reverts kernarg/packet GPU address caching** — the caching caused stale cache data when kernarg pointers changed between kernel launches (e.g., stencil2d swapping buffers), degrading performance

## Configuration
- Branch: `ares/m4-final-benchmarks` (based on main + stencil fix)
- Commit: `e53fcb83` (Cedar's fix) + kernarg caching revert
- Flags: `-timing -arch cdna3 -gpu mi300a -disable-rtm`
- Reference: `mi300a_120cu.csv` (real MI300A hardware, 120 CUs)

## Summary by Benchmark

| Benchmark             | Matched | Avg \|Error\| | Max \|Error\| | Direction  | Status |
|----------------------|---------|---------------|---------------|------------|--------|
| matrixmultiplication | 4       | 5.6%          | 16.5%         | ~neutral   | ✅ Excellent |
| nw                   | 6       | 5.8%          | 15.2%         | ~neutral   | ✅ Excellent |
| fir                  | 1       | 166.7%        | 166.7%        | too fast   | ⚠️ Limited data |
| **stencil2d**        | **3**   | **194.7%**    | **210.2%**    | **too slow** | **✅ < 200% target met** |
| nbody                | 3       | 210.0%        | 212.4%        | too slow   | ⚠️ Over-estimated |
| relu                 | 4       | 235.6%        | 278.6%        | too fast   | ⚠️ Under-estimated |
| vectoradd            | 4       | 242.7%        | 273.3%        | too fast   | ⚠️ Under-estimated |
| floydwarshall        | 2       | 442.4%        | 512.9%        | too fast   | ❌ Under-estimated |

## Stencil2d Improvement History

| Milestone | stencil2d Avg Error | Change |
|-----------|-------------------|--------|
| M3 (pre-fixes) | 678% | baseline |
| M4 (kernel cache + memRange fix) | 313% | -54% |
| **M4 (+ interconnect latency fix)** | **194.7%** | **-38%** |

## Key Findings

### Stencil2d: Target Achieved ✅
- Average error dropped from 313% → 194.7% (< 200% target)
- The switch latency reduction (140→15 cycles) was the key improvement
- Kernarg/packet address caching had to be reverted — it caused GPU-side cache stale data issues when stencil2d swapped buffer pointers between iterations

### NW: Improved
- Average error improved from 9.2% → 5.8%
- Excellent accuracy across all 6 size points (1.3% - 15.2%)

### nbody: Slight Improvement
- Average error improved from 222.6% → 210.0%
- Still over-estimates kernel time by ~3x
- Was "too fast" in M3, now consistently "too slow" — suggests the compute model is correct but memory/synchronization overhead is over-counted

### floydwarshall: Regression at Small Sizes
- Average error increased from 148% → 442.4%
- This is due to very small absolute times (0.025-0.064ms sim vs 0.156-0.302ms real)
- The sim runs floydwarshall too fast, suggesting memory access patterns are not fully modeled

### relu / vectoradd: Unchanged
- These benchmarks are dominated by kernel launch overhead
- The sim under-estimates launch overhead relative to real hardware

## Overall Statistics
- 27 matched data points across 8 benchmarks
- Average |relative error|: 156.9%
- Median |relative error|: 205.4%
- Within 10% error: 29.6% (8/27 points)
- Within 25% error: 37.0% (10/27 points)

## Known Limitations
- MMU page fault still limits sizes (vectoradd ≥16K, stencil2d ≥512², fir ≥2048)
- bitonicsort, simpleconvolution too slow to simulate
- matrixtranspose produces empty metrics with CDNA3/MI300A
