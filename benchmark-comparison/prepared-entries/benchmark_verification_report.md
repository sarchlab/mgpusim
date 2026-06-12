# Benchmark Verification Report — Issue #169

## Task 1: Verify existing broken benchmark entries

### Summary
- **11 of 13 benchmarks**: All fields correct (sizes, flag_template, size_label_template, CSV_NAME)
- **2 benchmarks with issues**: bfs, kmeans

### Detailed Results

| Benchmark | Sizes | flag_template | size_label_template | CSV_NAME | Status |
|-----------|-------|---------------|---------------------|----------|--------|
| bfs | ❌ 5 of 18 node sizes | ✅ `-node {p1} -degree {p2} -depth 100` | ✅ `{p1}` | ✅ `bfs` | **MISMATCH** |
| ga | ✅ 16/16 | ✅ `-population-size {size}` | ✅ `{size}` | ✅ `ga` | OK |
| gramschmidt | ✅ 10/10 | ✅ `-n {size} -m {size}` | ✅ `{size}` | ✅ `gramschmidt` | OK |
| hist | ✅ 18/18 | ✅ `-num-elements {size} -num-bins 256` | ✅ `{size}` | ✅ `hist` | OK |
| huffman | ✅ 16/16 | ✅ `-data_size {size}` | ✅ `{size}` | ✅ `huffman` | OK |
| kmeans | ❌ 5 of 16 point sizes | ✅ `-points {p1} -features {p2} -clusters 5 -max-iter 1` | ❌ `{p1}_f{p2}_i1` (HW uses just `{p1}`) | ✅ `kmeans` | **MISMATCH** |
| lud | ✅ 18/18 | ✅ `-size {size}` | ✅ `{size}` | ✅ `lud` | OK |
| particlefilter | ✅ 16/16 | ✅ `-num_particles {size}` | ✅ `{size}` | ✅ `particlefilter` | OK |
| parboil_lbm | ✅ 17/17 | ✅ `-grid_dim {size}` | ✅ `{size}` | ✅ `lbm_stream_collide` | OK |
| parboil_mri_gridding | ✅ 17/17 | ✅ `-num_samples {size}` | ✅ `{size}` | ✅ `gridding_kernel` | OK |
| parboil_sad | ✅ 17/17 | ✅ `-frame_width {size}` | ✅ `{size}` | ✅ `ComputeSAD` | OK |
| parboil_tpacf | ✅ 18/18 | ✅ `-num_points {size}` | ✅ `{size}` | ✅ `tpacf_DD` | OK |
| streamcluster | ✅ 16/16 | ✅ `-num_points {size} -num_dimensions 32 -num_centers 10` | ✅ `{size}` | ✅ `streamcluster_pgain` | OK |

### Issue Details

#### bfs — Missing 13 node sizes
Current YML has 5 node values: 1024, 4096, 16384, 65536, 262144 (each × 4 degrees)
HW reference has 18 node values: 1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072, 196608, 262144, 524288, 786432, 1048576, 1572864, 2097152, 2621440, 3145728, 4194304

Missing from YML: 2048, 8192, 32768, 131072, 196608, 524288, 786432, 1048576, 1572864, 2097152, 2621440, 3145728, 4194304

The `size_label_template: "{p1}"` is correct (matches HW format), but the compound sizes need expanding. Since HW only uses node count as problem_size, we should fix the sizes to include all 18 node values. The degree parameter choice doesn't affect matching — but we should pick a reasonable default (e.g., degree=8).

**Recommended fix**: Use single-param format with fixed degree, or expand compound format to cover all 18 node counts:
```yaml
sizes: "1024_8 2048_8 4096_8 8192_8 16384_8 32768_8 65536_8 131072_8 196608_8 262144_8 524288_8 786432_8 1048576_8 1572864_8 2097152_8 2621440_8 3145728_8 4194304_8"
```

#### kmeans — Missing 11 point sizes AND wrong size_label_template
Current YML has 5 point values: 1024, 4096, 16384, 65536, 262144 (each × 4 features)
HW reference has 16 point values: 1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072, 262144, 524288, 786432, 1048576, 1572864, 2097152, 3145728, 4194304

Missing from YML: 2048, 8192, 32768, 131072, 524288, 786432, 1048576, 1572864, 2097152, 3145728, 4194304

Additionally, `size_label_template: "{p1}_f{p2}_i1"` generates labels like `1024_f8_i1`, but HW reference uses just `1024`. This means even sizes that DO run won't match the reference data.

**Recommended fix**: Use single-param format with fixed features, and fix label:
```yaml
sizes: "1024 2048 4096 8192 16384 32768 65536 131072 262144 524288 786432 1048576 1572864 2097152 3145728 4194304"
flag_template: "-points {size} -features 32 -clusters 5 -max-iter 1"
size_label_template: "{size}"
```

---

## Task 3: HW reference kernels NOT in benchmark.yml

### 12 Microbenchmarks (Task 2 — entries prepared)
| Kernel | # Sizes | Notes |
|--------|---------|-------|
| fp32_fma | 16 | Compute throughput |
| fp64_fma | 16 | Compute throughput |
| int_mad | 16 | Compute throughput |
| sfun_sin | 15 | Compute throughput |
| occupancy_fma | 16 | Compute throughput |
| global_bw_copy | 16 | Memory bandwidth |
| shared_bw | 16 | Memory bandwidth |
| l1_cache_bw | 16 | Cache bandwidth |
| l2_cache_bw | 16 | Cache bandwidth |
| mem_latency_chase | 16 | Memory latency |
| atomic_throughput | 11 | Atomic ops |
| branch_div_50pct | 16 | Branch divergence |

### 23 Other missing kernels (not microbenchmarks)
| Kernel | # Sizes | Likely Suite | Notes |
|--------|---------|-------------|-------|
| BusSpeedDownload | 19 | SHOC | Uses KB units (special format) |
| BusSpeedReadback | 19 | SHOC | Uses KB units (special format) |
| DeviceMemory_Read | 16 | SHOC | Uses MB units (special format) |
| DeviceMemory_Write | 16 | SHOC | Uses MB units (special format) |
| FindMinSAD | 17 | Parboil | Second kernel from parboil_sad (already included) |
| MaxFlops | 16 | SHOC | SHOC MaxFlops benchmark |
| adjust_weights | 18 | Rodinia | Second kernel from backprop (already included) |
| bh | 18 | Lonestar | Barnes-Hut |
| dmr | 15 | Lonestar | Delaunay Mesh Refinement |
| fused_swiglu | 18 | ML ops | Compound problem_size format |
| gelu | 1 | ML ops | Single data point |
| layernorm | 18 | ML ops | Compound problem_size format |
| md | 16 | Lonestar | Molecular Dynamics |
| md5hash | 16 | Unknown | MD5 hash |
| naive_attention | 17 | ML ops | Compound problem_size format |
| qtc | 16 | Lonestar | Quadrilateral intersection |
| rope | 16 | ML ops | Compound problem_size format |
| s3d | 16 | Parboil | S3D combustion |
| softmax | 18 | ML ops | Compound problem_size format |
| sssp | 18 | Lonestar | Single-Source Shortest Path |
| tiled_gemm_16 | 18 | ML ops | Tiled GEMM with tile=16 |
| tpacf_DR | 18 | Parboil | Second kernel from parboil_tpacf (already included) |
| tpacf_RR | 18 | Parboil | Third kernel from parboil_tpacf (already included) |

### Notes on multi-kernel benchmarks
- **parboil_sad** produces both `ComputeSAD` and `FindMinSAD` — the second kernel is NOT being captured
- **backprop** produces both `layerforward` and `adjust_weights` — the second kernel is NOT being captured
- **parboil_tpacf** produces `tpacf_DD`, `tpacf_DR`, and `tpacf_RR` — only DD is captured

These multi-kernel outputs require changes to the metric extraction logic (currently only extracts one `kernel_time`), not to benchmark.yml. This should be a separate issue.
