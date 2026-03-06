# MI300A CU Pipeline Fix Benchmark Results

**Branch:** `ares/m2-cu-pipeline-fixes`  
**Commit:** `08138f80` — [Finn] Implement CDNA3 SIMD width + VecMem pipeline depth changes  
**Date:** 2026-03-06  
**Hardware reference:** `gpu_perf_scripts/mi300a_120cu.csv` (real MI300A, 120 CU)  
**Flags:** `-timing -arch cdna3 -gpu mi300a -disable-rtm`

## Parameter Changes (3 total)

| # | Component | Parameter | GCN3 Default | CDNA3 Value | File |
|---|---|---|---|---|---|
| 1 | SIMD Unit | NumSinglePrecisionUnit | 16 | 32 | `cu/cubuilder.go` |
| 2 | VecMem Unit | Instruction Pipeline Stages | 6 | 2 | `cu/cubuilder.go` |
| 3 | VecMem Unit | Transaction Pipeline Stages | 10 (was 60*) | 4 | `cu/cubuilder.go` |

*Note: The transaction pipeline was already changed from 60→10 in a previous cache timing fix. This commit changes the GCN3 default from 10 back to the original 60 for the base builder, and sets the CDNA3 value to 4 via the MI300A config.

**Rationale:**
- **SIMD width 16→32:** CDNA3 SIMDs process 32 work-items per cycle (vs 16 in GCN3), halving the number of cycles needed to execute a wavefront's 64 work-items through the ALU.
- **VecMem instruction pipeline 6→2:** Reduces instruction decode/issue overhead for memory operations, reflecting CDNA3's simpler memory pipeline.
- **VecMem transaction pipeline 10→4:** Reduces per-transaction pipeline latency for memory requests.

## Benchmark Results

### Comparison: Before Fix vs After Fix vs Real Hardware

| Benchmark | Before (ms) | After (ms) | Sim Change | HW (ms) | Before Err | After Err | Accuracy Δ |
|---|---|---|---|---|---|---|---|
| matmul 128³ | 0.0345 | 0.0282 | −18.2% | 0.0243 | +42.0% | +16.2% | **+25.8pp** ✅ |
| matmul 256³ | 0.0884 | 0.0772 | −12.7% | 0.0403 | +119.4% | +91.5% | **+27.8pp** ✅ |
| kmeans 1024p/32f/5c | 0.0502* | 0.0991 | +97.4% | 0.0177 | +183.7% | +460.0% | −276.3pp ❌ |
| floydwarshall 64 | 0.1830 | 0.1585 | −13.4% | 0.3024 | −39.5% | −47.6% | −8.1pp ❌ |
| nw 128 | 0.1433* | 0.1183 | −17.5% | 0.1285 | +11.5% | −8.0% | **+3.5pp** ✅ |

*Before values for kmeans and nw estimated from known error percentages stated in issue #260.

### Detailed Results (Raw Simulator Output)

| Benchmark | Problem Size | Sim kernel_time (s) | Sim (ms) | Real HW (ms) | Error (%) |
|---|---|---|---|---|---|
| matmul | 128×128×128 | 2.8237e-05 | 0.0282 | 0.0243 | +16.2% |
| matmul | 256×256×256 | 7.7192e-05 | 0.0772 | 0.0403 | +91.5% |
| kmeans | 1024 pts, 32 feat, 5 clusters | 9.9113e-05 | 0.0991 | 0.0177 | +460.0% |
| floydwarshall | 64 nodes | 1.5851e-04 | 0.1585 | 0.3024 | −47.6% |
| nw | length=128 | 1.1827e-04 | 0.1183 | 0.1285 | −8.0% |

## Analysis

### Compute-Bound Benchmarks (matmul, nw)

The SIMD width doubling (16→32) significantly improves accuracy for compute-bound workloads:

- **matmul 128³:** Error reduced from +42.0% to +16.2% (25.8 percentage point improvement). The simulator was previously too slow because it was taking 4 cycles per wavefront instruction instead of the correct 2 cycles.
- **matmul 256³:** Error reduced from +119.4% to +91.5% (27.8pp improvement). Still high, likely due to remaining compute pipeline modeling issues at larger sizes.
- **nw 128:** Error went from +11.5% to −8.0% (3.5pp improvement in absolute error). The sign flip from positive to negative indicates the simulator now slightly underpredicts execution time, suggesting the fix slightly overcorrects for this mixed compute/memory workload.

### Memory-Bound Benchmarks (floydwarshall)

- **floydwarshall 64:** Error worsened from −39.5% to −47.6% (8.1pp regression). The simulator was already too fast for this memory-intensive benchmark, and reducing pipeline depths makes it even faster. This indicates other memory subsystem bottlenecks (e.g., memory bandwidth, cache miss handling) are undermodeled.

### Kmeans (Divergent Compute)

- **kmeans:** Error increased dramatically from +183.7% to +460.0%. The kmeans benchmark involves iterative kernel launches (5 iterations) with divergent branching. The wider SIMD (32 SPUs) may expose issues with how wavefront divergence is modeled — if branch divergence causes repeated re-execution, wider SIMDs can amplify the slowdown. This regression warrants further investigation.

### Error Direction Summary

| Benchmark | Category | Error Direction | Meaning |
|---|---|---|---|
| matmul 128³ | Compute-bound | Positive (+16.2%) | Sim too slow |
| matmul 256³ | Compute-bound | Positive (+91.5%) | Sim too slow |
| kmeans | Divergent compute | Positive (+460.0%) | Sim way too slow |
| floydwarshall | Memory-bound | Negative (−47.6%) | Sim too fast |
| nw | Mixed | Negative (−8.0%) | Sim slightly too fast |

### Cumulative Effect of All Timing Fixes

Including DRAM fix, cache timing fix, and this CU pipeline fix:

| Benchmark | Original Sim (ms)¹ | All Fixes (ms) | Total Change | HW (ms) |
|---|---|---|---|---|
| matmul 128³ | 0.0451 | 0.0282 | −37.5% | 0.0243 |
| matmul 256³ | 0.2089 | 0.0772 | −63.0% | 0.0403 |
| floydwarshall 64 | 0.1678² | 0.1585 | −5.5% | 0.3024 |

¹Original = pre-DRAM-fix baseline  
²floydwarshall original from `docs/mi300a_dram_fix_benchmarks.md`
