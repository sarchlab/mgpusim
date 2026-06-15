# MI300A M6 Benchmark Results

## Configuration
- **Branch:** main
- **Commit:** d0020901 (`[Ares] Fix FLAT SAddr mode in timing: add scalar base + signed offset to addresses`)
- **Config:** `-timing -arch cdna3 -gpu mi300a -disable-rtm`
- **Date:** 2026-03-06

## Key M6 Changes
1. **FLAT SAddr mode fix** (d0020901): Fixed scalar base + signed offset addressing in timing mode
2. **vectoradd width>4096 fix** (49c025f7): V5 kernel register count + OOO inst fetch
3. **Out-of-order memory response fix** (04cd88e0): Fixed memory response handling in compute unit
4. **VCCLO write mask inversion fix** (63770491): Corrected VCCLO write mask

## Results Summary

### vectoradd
| Size | Sim kernel_time (s) | Real avg (ms) | Real avg (s) | Relative Error | Status |
|------|---------------------|---------------|---------------|----------------|--------|
| 1,024 | 3.425e-06 | 0.0043 | 4.3e-06 | -20.3% | ✅ |
| 4,096 | 3.484e-06 | 0.0050 | 5.0e-06 | -30.3% | ✅ |
| 16,384 | 3.716e-06 | 0.0056 | 5.6e-06 | -33.6% | ✅ |
| 65,536 | 5.571e-06 | 0.0048 | 4.8e-06 | +16.1% | ✅ |
| 262,144 | 1.4102e-05 | 0.0052 | 5.2e-06 | +171.2% | ✅ |
| 524,288 | 2.5481e-05 | 0.0061 | 6.1e-06 | +317.7% | ✅ |
| 1,048,576 | 4.8237e-05 | 0.0069 | 6.9e-06 | +599.1% | ✅ |
| 2,097,152 | — | 0.0107 | 1.07e-05 | — | ⏱️ sim too slow |
| 4,194,304 | — | 0.0166 | 1.66e-05 | — | ⏱️ sim too slow |

**Verification:** vectoradd width=16384 passes with `-verify` ✅

**Note:** At small sizes (1K-16K), sim is within ~30% of real hardware. At larger sizes (262K+), sim kernel time grows much faster than real hardware, suggesting the simulator's per-element compute cost is too high relative to real hardware which benefits from high memory bandwidth and compute throughput.

### atax (Scaling Test)
| Size | Sim kernel_time (s) | Real avg (ms) | Real avg (s) | Relative Error | Status |
|------|---------------------|---------------|---------------|----------------|--------|
| 32×32 | 1.1738e-05 | — | — | — | ✅ (no ref data) |
| 48×48 | — | 0.0242 | 2.42e-05 | — | (not tested) |
| 64×64 | 2.1063e-05 | 0.0293 | 2.93e-05 | -28.1% | ✅ |
| 128×128 | 6.299e-05 | 0.0479 | 4.79e-05 | +31.5% | ✅ |
| 256×256 | 2.78017e-04 | 0.1504 | 1.504e-04 | +84.8% | ✅ |
| 512×512 | 9.55943e-04 | 0.2691 | 2.691e-04 | +255.2% | ✅ |
| 768×768 | 8.24989e-04 | 0.4323 | 4.323e-04 | +90.8% | ✅ |
| 1024×1024 | — | 0.5804 | 5.804e-04 | — | ⏱️ sim too slow |

**Note:** atax now runs successfully at all tested sizes up to 768×768. Error grows at mid-sizes but is reasonable. The 768 result being faster than 512 is unexpected and may warrant investigation.

### stencil2d
| Size | Sim kernel_time (s) | Real avg (ms) | Real avg (s) | Relative Error | Status |
|------|---------------------|---------------|---------------|----------------|--------|
| 64×64 | 2.8294e-05 | 0.0059 | 5.9e-06 | +379.6% | ✅ |
| 128×128 | 2.8689e-05 | 0.0062 | 6.2e-06 | +362.7% | ✅ |
| 256×256 | 2.7678e-05 | 0.0065 | 6.5e-06 | +325.8% | ✅ |
| 512×512 | 2.977e-05 | 0.0063 | 6.3e-06 | +372.5% | ✅ |
| 768×768 | 3.7236e-05 | 0.0071 | 7.1e-06 | +424.5% | ✅ |
| 1024×1024 | — | 0.0111 | 1.11e-05 | — | ❌ MMU crash |

**Note:** stencil2d kernel time is relatively flat (~28-37 µs) while real hardware shows increasing times. This suggests the simulator is compute-bound at a constant rate while real hardware is scaling properly. The sim is 3-5x slower than real hardware at all sizes.

### bicg
| Size | Sim kernel_time (s) | Real avg (ms) | Real avg (s) | Relative Error | Status |
|------|---------------------|---------------|---------------|----------------|--------|
| 64×64 | 2.3634e-05 | 0.0245 | 2.45e-05 | -3.5% | ✅ |
| 128×128 | 6.8151e-05 | 0.0470 | 4.70e-05 | +45.0% | ✅ |
| 256×256 | 2.88062e-04 | 0.1274 | 1.274e-04 | +126.1% | ✅ |
| 512×512 | 9.74904e-04 | 0.2689 | 2.689e-04 | +262.5% | ✅ |
| 768×768 | 8.55847e-04 | 0.4196 | 4.196e-04 | +103.9% | ✅ |

**Note:** bicg at 64×64 is within 3.5% — excellent match. Error grows at larger sizes, with the same anomaly at 768 being faster than 512 (same as atax, since bicg is a similar polybench kernel).

### spmv
| Config | Sim kernel_time (s) | Real avg (ms) | Real avg (s) | Relative Error | Status |
|--------|---------------------|---------------|---------------|----------------|--------|
| dim=1024, sp=0.004 (~4194 nnz) | 5.964e-06 | 0.0058 | 5.8e-06 | +2.8% | ✅ |
| dim=4096, sp=0.001 (~16777 nnz) | 8.624e-06 | 0.0058 | 5.8e-06 | +48.7% | ✅ |
| dim=4096, sp=0.004 (~67108 nnz) | 1.8104e-05 | 0.0068 | 6.8e-06 | +166.2% | ✅ |
| dim=16384, sp=0.004 | — | — | — | — | ❌ crash/timeout |

**Note:** spmv at dim=1024 is within 3% — excellent match. Error grows with larger matrices.

### fft
| Size | Sim kernel_time (s) | Real avg (ms) | Real avg (s) | Relative Error | Status |
|------|---------------------|---------------|---------------|----------------|--------|
| 1MB (~131072 elements) | 4.0077e-05 | 0.0083 | 8.3e-06 | +382.9% | ✅ |
| 2MB (~262144 elements) | 4.2328e-05 | 0.0092 | 9.2e-06 | +360.1% | ✅ |
| 4MB | — | — | — | — | ⏱️ sim too slow |

**Note:** FFT simulation is ~4x slower than real hardware. The kernel overhead dominates.

### nbody
| Size | Sim kernel_time (s) | Status |
|------|---------------------|--------|
| 64 particles | — | ❌ MMU "page not found" crash |
| 256 particles | — | ❌ MMU "page not found" crash |
| 1024 particles | — | ❌ MMU "page not found" crash |

**Note:** nbody crashes at all sizes with the known MMU bug (akita/v4/mem/vm/mmu/mmu.go:107 finalizePageWalk). This is a pre-existing issue.

### gesummv, gemm, mvt
These benchmarks do **not exist** in `amd/samples/`. They are not currently implemented in MGPUSim.

## Error Summary (matched data points only)

| Benchmark | Sizes Tested | Best Error | Worst Error | Avg |Error| |
|-----------|-------------|------------|-------------|-------------|
| vectoradd | 7 | -20.3% (1K) | +599.1% (1M) | 170% |
| atax | 4 | -28.1% (64²) | +255.2% (512²) | 100% |
| stencil2d | 5 | +325.8% (256²) | +424.5% (768²) | 373% |
| bicg | 5 | -3.5% (64²) | +262.5% (512²) | 108% |
| spmv | 3 | +2.8% (1024) | +166.2% (4096) | 73% |
| fft | 2 | +360.1% (2MB) | +382.9% (1MB) | 372% |
| nbody | 0 | — | — | ❌ all crash |

**Overall mean |relative error|:** ~196% across 26 matched data points

## Comparison with M5

| Metric | M5 | M6 |
|--------|-----|-----|
| Matched data points | 42 | 26 |
| Mean |relative error| | 120.1% | ~196% |
| Median |relative error| | 55.6% | ~126% |
| Best individual errors | nw (5.8%), spmv (9.6%) | spmv (2.8%), bicg (3.5%) |
| Worst | atax (542.7%) | vectoradd (599.1%) |
| nbody | Crashed | Crashed |
| atax | Works (high error) | Works (improved at small sizes) |
| vectoradd large | Not tested | Works but slow sim |

**Key changes from M5:**
1. **vectoradd with width>4096 now works** (was broken in M5)
2. **FLAT SAddr mode fixed** — some benchmarks may be more accurate
3. **VCCLO fix** — affects conditional operations
4. **MMU page-not-found bug persists** — blocks nbody at all sizes, stencil2d at 1024+

## Known Issues
1. **MMU page-not-found panic** (pre-existing): Affects nbody (all sizes), stencil2d (≥1024), large spmv
2. **Simulation speed**: Larger problem sizes take very long wall-clock time (>3 min for moderate sizes)
3. **Kernel overhead**: constantKernelOverhead of 3600 ticks dominates at small problem sizes
4. **Missing benchmarks**: gesummv, gemm, mvt not implemented
