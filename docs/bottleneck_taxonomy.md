# GPU Bottleneck Taxonomy

This document maps each of the 79 matched benchmark kernels in
`benchmark-comparison/comparison_ci.csv` to one of 12 GPU bottleneck
categories and assesses how well MGPUSim models each category.

**Data source:** CI run 24043266057 — 495 matched data points, 79 kernels.

Kernels flagged with 🔬 are **microbenchmarks** (as defined by the
`MICROBENCHMARKS` set in `workloads/scripts/compare_sim_vs_real.py`).

---

## Summary Table

| # | Category | Kernels | Matched | Points | Median \|err\| | Mean \|err\| | Within 25% | Grade |
|---|----------|---------|---------|--------|---------------|-------------|------------|-------|
| 1 | Compute-bound (ALU-heavy) | 14 | 14 | 83 | 87.7% | 67.0% | 30% | ❌ Bad |
| 2 | Memory-bound (bandwidth) | 9 | 9 | 75 | 80.3% | 172.4% | 13% | ❌ Bad |
| 3 | Memory-bound (latency) | 2 | 2 | 19 | 99.9% | 94.5% | 0% | ❌ Bad |
| 4 | Stencil/convolution | 7 | 7 | 38 | 48.7% | 52.6% | 21% | ⚠️ Poor |
| 5 | Reduction | 7 | 7 | 40 | 23.5% | 33.1% | 50% | 🟡 Fair |
| 6 | Scan/prefix | 1 | 1 | 8 | 41.2% | 41.3% | 0% | ⚠️ Poor |
| 7 | Graph/irregular | 7 | 7 | 41 | 98.2% | 96.9% | 0% | ❌ Bad |
| 8 | Sort/shuffle | 4 | 4 | 37 | 50.9% | 54.5% | 38% | ⚠️ Poor |
| 9 | FFT | 0 | 0 | 0 | — | — | — | N/A |
| 10 | Image/video | 2 | 2 | 2 | 21.3% | 21.3% | 50% | 🟡 Fair |
| 11 | Scientific simulation | 19 | 19 | 114 | 39.7% | 70.9% | 31% | 🟡 Fair |
| 12 | ML/AI | 8 | 7 | 35 | 74.8% | 430.6% | 34% | ❌ Bad |

**Grading scale** (by median |relative error|):
✅ Good ≤ 15% · 🟡 Fair ≤ 40% · ⚠️ Poor ≤ 70% · ❌ Bad > 70%

---

## 1. Compute-bound (ALU-heavy)

**Description:** Kernels whose runtime is dominated by arithmetic/logic
operations — integer, single/double-precision FP, or transcendentals.
Includes dense linear-algebra (GEMM variants) where the compute-to-memory
ratio is high.

**Median |relative error|: 87.7%** · Grade: ❌ Bad

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| 2mm | 1.3% | 3 | Polybench matrix-multiply chain |
| 3mm | 15.1% | 2 | Polybench triple matrix multiply |
| branch_div_50pct 🔬 | 90.1% | 8 | Branch divergence microbenchmark |
| fp32_fma 🔬 | 97.1% | 8 | Peak FP32 throughput test |
| fp64_fma 🔬 | 96.8% | 8 | Peak FP64 throughput test |
| gemm | 9.7% | 3 | SHOC GEMM |
| int_mad 🔬 | 99.4% | 10 | Integer multiply-add throughput |
| maxflops | 21.6% | 9 | Maximum FLOPS test |
| occupancy_fma | 45.5% | 8 | Occupancy sweep |
| sfun_sin 🔬 | 99.9% | 10 | Transcendental (sin) throughput |
| sgemm_tiled | 18.8% | 5 | Parboil tiled SGEMM |
| syr2k | 69.7% | 2 | Polybench SYR2K |
| syrk | 70.7% | 3 | Polybench SYRK |
| tiled_gemm_16 | 7.2% | 4 | Custom 16×16 tiled GEMM |

**Analysis:** The category median is heavily skewed by microbenchmarks that
test raw peak throughput (`fp32_fma`, `fp64_fma`, `int_mad`, `sfun_sin`).
The simulator models ~7–9% of real hardware peak, making these
near-100% underestimates. If microbenchmarks are excluded, the
application-level GEMM kernels (2mm, gemm, sgemm_tiled, tiled_gemm_16)
achieve median errors of 7–19%, which is quite good.

**Known limitations:**
- Functional-unit throughput is not cycle-accurate; the sim achieves
  ~7–9% of real ALU peak → catastrophic error on pure throughput tests.
- Branch divergence handling (`branch_div_50pct`) also far from reality.
- Application GEMMs with realistic memory traffic are modeled well because
  the memory system dominates total time.

---

## 2. Memory-bound (bandwidth)

**Description:** Kernels that stress sustained memory bandwidth — global,
L1, L2, shared, or host↔device transfers.

**Median |relative error|: 80.3%** · Grade: ❌ Bad

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| busspeeddownload 🔬 | 71.2% | 15 | Host→device bandwidth |
| busspeedreadback 🔬 | 77.2% | 15 | Device→host bandwidth |
| devicememory_read | 2876.1% | 2 | Raw device-memory read BW |
| devicememory_write | 82.8% | 8 | Raw device-memory write BW |
| global_bw_copy | 461.5% | 3 | Global memory copy BW |
| l1_cache_bw | 88.4% | 16 | L1 cache bandwidth sweep |
| l2_cache_bw | 69.0% | 2 | L2 cache bandwidth |
| shared_bw | 26.3% | 1 | Shared memory bandwidth |
| triad | 18.3% | 13 | STREAM triad |

**Analysis:** Only `triad` (18%) and `shared_bw` (26%) show reasonable
accuracy. Raw bandwidth microbenchmarks (`devicememory_read`,
`global_bw_copy`) are extremely off, indicating the memory controller
and cache hierarchy do not reproduce real sustained bandwidth.

**Known limitations:**
- DMA/PCIe transfer model is simplistic → bus-speed tests wrong.
- Global-memory controller does not model bank-level parallelism or
  GDDR6 burst scheduling, so raw BW tests overshoot or undershoot
  dramatically.
- `triad` works better because its compute/memory balance masks
  bandwidth inaccuracies.

---

## 3. Memory-bound (latency)

**Description:** Kernels that stress memory-access latency — pointer
chasing, atomic contention.

**Median |relative error|: 99.9%** · Grade: ❌ Bad

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| atomic_throughput | 100.0% | 10 | Atomic operation throughput |
| mem_latency_chase 🔬 | 87.2% | 9 | Pointer-chase latency |

**Analysis:** Both kernels are nearly 100% underestimated. The simulator
does not model atomic contention or true cache-miss latency chains.

**Known limitations:**
- Atomic operations use a simplified memory-ordering model.
- Cache-miss latency is fixed rather than dynamically modeled.

---

## 4. Stencil/convolution

**Description:** Regular-grid stencil computations and convolutions with
predictable neighbor-access patterns.

**Median |relative error|: 48.7%** · Grade: ⚠️ Poor

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| 2dconvolution | 32.5% | 10 | Polybench 2-D convolution |
| 3dconvolution | 25.5% | 2 | Polybench 3-D convolution |
| fdtd2d | 66.2% | 4 | Polybench FDTD-2D |
| hotspot | 54.3% | 8 | Rodinia hotspot |
| hotspot3d | 44.7% | 4 | Rodinia hotspot3D |
| srad | 70.7% | 4 | Rodinia SRAD |
| stencil_kernel | 35.2% | 6 | Parboil stencil |

**Analysis:** Moderate accuracy. The regular access pattern is partially
captured, but the simulator over-predicts execution time for most
stencils (too slow by 30–70%). This likely reflects inaccurate cache-line
reuse and prefetch modeling for strided access patterns.

**Known limitations:**
- No hardware prefetcher model.
- L1/L2 cache replacement policy may not match real Vega hardware.

---

## 5. Reduction

**Description:** Kernels dominated by parallel reduction patterns —
tree-reduction, dot products, matrix–vector products with accumulation.

**Median |relative error|: 23.5%** · Grade: 🟡 Fair

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| atax | 9.2% | 5 | Polybench ATAX (gate benchmark) |
| bicg | 6.1% | 5 | Polybench BiCG (gate benchmark) |
| correlation | 59.4% | 4 | Polybench correlation |
| covariance | 62.7% | 4 | Polybench covariance |
| gesummv | 80.7% | 4 | Polybench GESUMMV |
| mvt | 32.3% | 5 | Polybench MVT |
| reduction | 9.4% | 13 | HeteroMark reduction |

**Analysis:** This is one of the best-modeled categories. Simple
reductions (`atax`, `bicg`, `reduction`) achieve <10% error. More complex
kernels that combine reduction with other patterns (`correlation`,
`covariance`, `gesummv`) show higher error, likely due to multi-pass
memory traffic that the sim doesn't capture as accurately.

**Known limitations:**
- Warp-level reduction intrinsics (e.g. `__shfl_down`) may not be
  faithfully timed.
- `gesummv` likely bottlenecked on a memory pattern the sim mismodels.

---

## 6. Scan/prefix

**Description:** Parallel prefix-sum (scan) kernels with work-efficient
up/down sweep patterns.

**Median |relative error|: 41.2%** · Grade: ⚠️ Poor

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| scan | 41.2% | 8 | HeteroMark prefix scan |

**Analysis:** Moderate error. Scan's bank-conflict-sensitive shared-memory
access pattern is only partially modeled.

**Known limitations:**
- Shared-memory bank conflicts not fully modeled.
- Multi-phase kernel launch overhead may differ from hardware.

---

## 7. Graph/irregular

**Description:** Kernels with irregular, data-dependent memory access
patterns — graph traversal, sparse matrix operations, adaptive meshing.

**Median |relative error|: 98.2%** · Grade: ❌ Bad

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| bfs | 98.5% | 12 | Rodinia BFS |
| bh | 98.9% | 7 | LoneStar BH (Barnes–Hut) |
| dmr | 95.8% | 2 | LoneStar DMR (Delaunay mesh refinement) |
| nw | 52.1% | 4 | Rodinia Needleman–Wunsch |
| pagerank | 90.7% | 3 | PageRank |
| spmv_csr | 45.6% | 6 | Sparse matrix-vector multiply (CSR) |
| sssp | 99.2% | 7 | LoneStar SSSP |

**Analysis:** The worst-modeled category overall. Nearly all graph
kernels show ~95–99% underestimation. `spmv_csr` (45.6%) is the best in the category — its CSR
access pattern is more predictable than true graph traversal. `nw` (52%) has some regularity in its DP wavefront.

**Known limitations:**
- The simulator severely underestimates graph-traversal time, likely due
  to missing TLB/page-table overhead, inaccurate cache-miss penalties
  for random access, and lack of work-distribution overhead modeling.
- Atomic-heavy graph algorithms (BFS, SSSP) compound the atomic modeling
  issue from Category 3.

---

## 8. Sort/shuffle

**Description:** Comparison-based and histogram-based sorting,
data-rearrangement, and encoding kernels.

**Median |relative error|: 50.9%** · Grade: ⚠️ Poor

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| hist | 18.5% | 14 | HeteroMark histogram |
| histogram | 33.4% | 6 | Parboil histogram |
| huffman | 90.6% | 11 | Rodinia Huffman coding |
| sort | 81.1% | 6 | HeteroMark bitonic sort |

**Analysis:** Histogramming kernels (`hist`, `histogram`) are modeled
fairly well thanks to their regular access pattern. Bitonic sort and
Huffman encoding involve complex data-movement and divergent control
flow that the sim struggles with.

**Known limitations:**
- Shared-memory shuffle patterns not accurately timed.
- Huffman encoding has serial dependencies poorly captured by the sim.

---

## 9. FFT

**Description:** Fast Fourier Transform kernels.

**No matched kernels in the current benchmark set.**

The `fft1d_512` kernel is defined in alias maps but did not produce
matched data points in the current CI run. This category cannot be
assessed.

---

## 10. Image/video

**Description:** Image search, motion estimation, and video-processing
kernels.

**Median |relative error|: 21.3%** · Grade: 🟡 Fair

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| computesad | 4.2% | 1 | Parboil SAD (sum of abs differences) |
| findminsad | 38.4% | 1 | Parboil find minimum SAD |

**Analysis:** Limited data (2 points total). `computesad` is well-modeled;
`findminsad` involves a reduction with data-dependent branches that adds
error. More data points are needed for a robust assessment.

**Known limitations:**
- Very small sample size limits confidence.

---

## 11. Scientific simulation

**Description:** Physics simulations, molecular dynamics, astronomy,
cryptography, and other scientific workloads.

**Median |relative error|: 39.7%** · Grade: 🟡 Fair

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| bs | 6.2% | 12 | Black–Scholes (HeteroMark) |
| computeq | 52.7% | 6 | Parboil MRI-Q |
| cutoff_potential | 10.1% | 1 | Parboil CUTCP |
| ep | 37.2% | 9 | HeteroMark EP (embarrassingly parallel) |
| ga | 98.3% | 16 | HeteroMark genetic algorithm |
| gaussian | 48.5% | 5 | Rodinia Gaussian elimination |
| gramschmidt | 67.1% | 1 | Polybench Gram–Schmidt |
| gridding_kernel | 48.2% | 7 | Parboil MRI gridding |
| lavamd | 11.0% | 1 | Rodinia LavaMD |
| lud | 34.5% | 5 | Rodinia LU decomposition |
| md | 20.7% | 4 | Rodinia molecular dynamics |
| md5hash | 8.7% | 8 | MD5 hash compute |
| nn | 29.6% | 12 | Rodinia nearest neighbor |
| particlefilter | 80.9% | 7 | Rodinia particle filter |
| pathfinder | 12.5% | 6 | Rodinia Pathfinder (DP) |
| qtc | 35.0% | 8 | Quality threshold clustering |
| tpacf_dd | 58.2% | 2 | Parboil TPACF (DD pairs) |
| tpacf_dr | 64.0% | 2 | Parboil TPACF (DR pairs) |
| tpacf_rr | 65.1% | 2 | Parboil TPACF (RR pairs) |

**Analysis:** Highly varied. Well-modeled kernels (`bs` 6%, `md5hash` 9%,
`cutoff_potential` 10%, `pathfinder` 13%) tend to have regular,
predictable access patterns. Poorly-modeled kernels (`ga` 98%,
`particlefilter` 81%) involve stochastic or irregular computation.
The TPACF kernels (58–65%) likely suffer from memory-access pattern
mismatch.

**Known limitations:**
- `ga` (genetic algorithm) uses heavy randomization and atomic operations.
- `particlefilter` has irregular, data-dependent memory access.
- TPACF kernels have complex trigonometric + histogramming patterns.

---

## 12. ML/AI

**Description:** Machine-learning inference and training primitives —
forward/backward passes, activation functions, attention, normalization.

**Median |relative error|: 74.8%** · Grade: ❌ Bad

| Kernel | Median \|err\| | Points | Notes |
|--------|---------------|--------|-------|
| adjust_weights | 1249.9% | 5 | Backprop weight update |
| fused_swiglu | 72.3% | 6 | Fused SwiGLU activation |
| gelu | — | 0 | Not matched (no sim data) |
| kmeans | 97.8% | 6 | K-means clustering |
| layerforward | 22.7% | 5 | Backprop forward pass |
| layernorm | 43.2% | 4 | Layer normalization |
| rope | 42.2% | 4 | Rotary position embedding |
| softmax | 11.5% | 5 | Softmax activation |

**Analysis:** The category median is inflated by `adjust_weights` (1250%
overestimate). Simple element-wise activations (`softmax` 12%,
`layerforward` 23%) are modeled reasonably. More complex fused
operations and iterative algorithms (`kmeans`, `fused_swiglu`) show
large errors.

**Known limitations:**
- `adjust_weights` is dominated by small-kernel launch overhead that
  the sim grossly over-predicts.
- `gelu` produced no matched data (sim may have timed out or been
  excluded from CI).
- `kmeans` involves iterative convergence with varying work per step.

---

## Categories the Simulator Models Well vs. Poorly

### Well-modeled (median |error| ≤ 40%)

| Category | Median \|err\| | Key insight |
|----------|---------------|-------------|
| **Reduction** | 23.5% | Simple parallel reductions with predictable memory access are the sim's sweet spot. Gate benchmarks `atax` (9%) and `bicg` (6%) fall here. |
| **Image/video** | 21.3% | Limited data, but SAD computation is regular and well-captured. |
| **Scientific simulation** | 39.7% | Mixed bag; regular physics kernels (bs, md, pathfinder) are good; stochastic/irregular ones (ga, particlefilter) are poor. |

### Poorly-modeled (median |error| > 70%)

| Category | Median \|err\| | Root cause |
|----------|---------------|------------|
| **Memory-bound (latency)** | 99.9% | Atomic and pointer-chase latency not modeled. |
| **Graph/irregular** | 98.2% | Random-access memory, TLB overhead, atomic contention all missing. |
| **Compute-bound (ALU-heavy)** | 87.7% | Peak throughput ~7–9% of real HW; microbenchmarks dominate. App-level GEMMs are actually good (7–19%). |
| **Memory-bound (bandwidth)** | 80.3% | Memory controller and DMA model too simplistic. |
| **ML/AI** | 74.8% | Outlier `adjust_weights` (1250%); element-wise ops are fair. |

### Structural observations

1. **Microbenchmarks inflate category errors.** Categories 1–3 each
   contain microbenchmarks that test raw hardware features the sim does
   not reproduce. Excluding microbenchmarks, the compute-bound app
   kernels achieve ~15% median error.

2. **Regular access patterns → good accuracy.** Across all categories,
   kernels with predictable, strided memory access (reductions, stencils,
   dense LA) are modeled much better than those with irregular, data-
   dependent access (graph traversal, particle filters, genetic algorithms).

3. **Atomic operations are a cross-cutting weakness.** Categories 3, 7,
   and 12 all suffer from inaccurate atomic modeling.

4. **No FFT data.** The FFT category has zero matched kernels and cannot
   be assessed. Adding `fft1d_512` to the CI benchmark suite would
   fill this gap.

---

## Unmatched Kernels (6 of 85)

These kernels have sim configurations but produced no matched data:

| Kernel | Reason |
|--------|--------|
| dwt2d | Timing sim deadlock |
| streamcluster_pgain | Timing sim deadlock |
| lbm_stream_collide | Timing sim deadlock |
| naive_attention | Timeout at 7200s (S32 infeasible) |
| s3d | Timeout at 7200s |
| gelu | No matched data in comparison_ci.csv |

---

*Generated from `benchmark-comparison/comparison_ci.csv` (CI run 24043266057).*
*See also: `workloads/scripts/compare_sim_vs_real.py`, `docs/validation_report.md`.*
