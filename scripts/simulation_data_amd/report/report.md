# MI300X Calibration Report

Simulated vs. real-hardware **kernel execution time** for the MI300X (CDNA3) timing model. One figure per (benchmark, non-scaling combo); x axis is the scaling factor, y axis is kernel time (log-log). The sim is **aligned** to real HW with a constant `offset` (= real − sim at the smallest shared scaling factor, the anchor). Errors are RELATIVE over the shared points **excluding the anchor**: `MAPE` = mean |sim−hw|/hw, `sMAPE` = mean |sim−hw|/min(sim,hw). `k (ref)` is the geomean sim/real ratio per figure — a reference for the multiplicative offset, not a score.

| Benchmark | Non-scaling combo | MAPE | sMAPE | k (ref) | Matched pts |
|-----------|-------------------|-----:|------:|--------:|------------:|
| altis_cfd | block_size=256, precision=double | 6.4% | 7.0% | 0.43× | 10 |
| altis_cfd | block_size=256, precision=float | 7.4% | 7.9% | 0.43× | 10 |
| altis_cfd | block_size=256, precision=half | 30.6% | 30.6% | 0.43× | 10 |
| altis_raytracing | height=1024, spheres=256 | — | — | 1.49× | 0 |
| altis_raytracing | height=1024, spheres=64 | — | — | 1.32× | 0 |
| altis_raytracing | height=1024, spheres=8 | — | — | 1.49× | 0 |
| altis_raytracing | height=2048, spheres=256 | — | — | 1.50× | 0 |
| altis_raytracing | height=2048, spheres=64 | — | — | 1.48× | 0 |
| altis_raytracing | height=2048, spheres=8 | — | — | 1.47× | 0 |
| cache_latency | cacheline_bytes=64, measure_laps=4, rng_seed=42 | 32.2% | 32.2% | 1.07× | 11 |
| empty_kernel | block_size=1024 | 44.1% | 44.1% | 0.33× | 8 |
| empty_kernel | block_size=256 | 1.4% | 1.5% | 0.19× | 8 |
| empty_kernel | block_size=32 | 34.5% | 100.4% | 0.09× | 8 |
| empty_kernel | block_size=64 | 34.6% | 100.6% | 0.09× | 8 |
| fp32_throughput | num_blocks=1, threads_per_block=1024 | 17.5% | 21.9% | 0.67× | 12 |
| fp32_throughput | num_blocks=1, threads_per_block=1 | 10.4% | 11.8% | 0.74× | 12 |
| fp32_throughput | num_blocks=1, threads_per_block=256 | 9.1% | 9.9% | 0.75× | 12 |
| fp32_throughput | num_blocks=1, threads_per_block=32 | 10.7% | 12.5% | 0.71× | 12 |
| fp32_throughput | num_blocks=1, threads_per_block=64 | 11.6% | 12.4% | 0.74× | 12 |
| fp64_throughput | num_blocks=1, threads_per_block=1024 | 55.7% | 55.7% | 1.60× | 12 |
| fp64_throughput | num_blocks=1, threads_per_block=1 | 6.3% | 6.3% | 0.94× | 12 |
| fp64_throughput | num_blocks=1, threads_per_block=256 | 5.8% | 5.8% | 0.97× | 12 |
| fp64_throughput | num_blocks=1, threads_per_block=32 | 10.0% | 10.0% | 0.92× | 12 |
| fp64_throughput | num_blocks=1, threads_per_block=64 | 5.5% | 5.5% | 0.93× | 12 |
| int32_throughput | num_blocks=1, threads_per_block=1024 | 24.0% | 37132.2% | 0.77× | 12 |
| int32_throughput | num_blocks=1, threads_per_block=1 | 31.8% | 47.9% | 0.59× | 12 |
| int32_throughput | num_blocks=1, threads_per_block=256 | 29.7% | 44.1% | 0.60× | 12 |
| int32_throughput | num_blocks=1, threads_per_block=32 | 29.9% | 44.3% | 0.59× | 12 |
| int32_throughput | num_blocks=1, threads_per_block=64 | 29.5% | 43.8% | 0.60× | 12 |
| memory_bandwidth | transfer_type=d2d, use_pinned_memory=true | 6.9% | 6.9% | 0.31× | 1 |
| parboil_cutcp | block_size=128, cutoff_radius=12, grid_spacing=0.25 | 39.6% | 67.7% | 0.47× | 7 |
| parboil_cutcp | block_size=128, cutoff_radius=12, grid_spacing=0.5 | 15.9% | 19.2% | 0.79× | 7 |
| parboil_cutcp | block_size=128, cutoff_radius=12, grid_spacing=1 | 5.5% | 5.5% | 1.09× | 7 |
| parboil_cutcp | block_size=128, cutoff_radius=12, grid_spacing=2 | 8.4% | 8.4% | 1.14× | 6 |
| parboil_cutcp | block_size=128, cutoff_radius=20, grid_spacing=0.25 | 42.1% | 75.8% | 0.39× | 6 |
| parboil_cutcp | block_size=128, cutoff_radius=20, grid_spacing=0.5 | 33.5% | 51.6% | 0.54× | 6 |
| parboil_cutcp | block_size=128, cutoff_radius=20, grid_spacing=1 | 8.9% | 10.0% | 0.89× | 6 |
| parboil_cutcp | block_size=128, cutoff_radius=20, grid_spacing=2 | 8.1% | 8.1% | 1.14× | 6 |
| parboil_cutcp | block_size=128, cutoff_radius=6, grid_spacing=0.25 | 15.3% | 18.4% | 0.79× | 6 |
| parboil_cutcp | block_size=128, cutoff_radius=6, grid_spacing=0.5 | 4.7% | 4.7% | 1.08× | 6 |
| parboil_cutcp | block_size=128, cutoff_radius=6, grid_spacing=1 | 8.8% | 8.8% | 1.15× | 6 |
| parboil_cutcp | block_size=128, cutoff_radius=6, grid_spacing=2 | 4.9% | 4.9% | 1.08× | 6 |
| parboil_lbm | block_size=128, iterations=1, num_timesteps=10, tau=0.7 | 30.9% | 30.9% | 0.73× | 2 |
| parboil_sgemm | block_size=256, precision=double | 30.1% | 30.1% | 1.28× | 4 |
| parboil_sgemm | block_size=256, precision=float | 30.8% | 30.8% | 1.30× | 4 |
| parboil_sgemm | block_size=256, precision=half | 29.9% | 29.9% | 1.28× | 4 |
| polybench_2dconv | block_size=256, precision=double | 10.3% | 10.3% | 0.51× | 6 |
| polybench_2dconv | block_size=256, precision=float | 11.9% | 13.2% | 0.48× | 6 |
| polybench_2dconv | block_size=256, precision=half | 16.8% | 16.8% | 0.50× | 6 |
| polybench_2mm | alpha=1.5, beta=1.2, block_size=256, precision=double | 17.7% | 17.7% | 1.04× | 5 |
| polybench_2mm | alpha=1.5, beta=1.2, block_size=256, precision=float | 23.1% | 23.1% | 1.04× | 5 |
| polybench_2mm | alpha=1.5, beta=1.2, block_size=256, precision=half | 17.0% | 17.0% | 1.03× | 5 |
| polybench_3dconv | block_size=8, filter_size=3 | 4.1% | 4.1% | 0.91× | 4 |
| polybench_3dconv | block_size=8, filter_size=5 | 8.1% | 8.1% | 0.94× | 4 |
| polybench_3dconv | block_size=8, filter_size=7 | 14.7% | 14.7% | 0.97× | 3 |
| polybench_3mm | block_size=256, precision=double | 35.0% | 35.0% | 1.49× | 4 |
| polybench_3mm | block_size=256, precision=float | 35.8% | 35.8% | 1.49× | 4 |
| polybench_3mm | block_size=256, precision=half | 39.8% | 39.8% | 1.48× | 4 |
| polybench_correlation | block_size=256, precision=double | 5.3% | 5.3% | 1.02× | 6 |
| polybench_correlation | block_size=256, precision=float | 5.9% | 5.9% | 1.03× | 6 |
| polybench_correlation | block_size=256, precision=half | 6.8% | 7.0% | 1.02× | 6 |
| polybench_fdtd2d | block_size=256, tmax=10 | 5.0% | 5.3% | 0.12× | 4 |
| polybench_fdtd2d | block_size=256, tmax=50 | — | — | 0.10× | 0 |
| polybench_gemm | block_size=256, precision=double | 53.8% | 53.8% | 1.31× | 4 |
| polybench_gemm | block_size=256, precision=float | 41.9% | 41.9% | 1.37× | 4 |
| polybench_gemm | block_size=256, precision=half | 41.1% | 41.1% | 1.38× | 4 |
| polybench_jacobi2d | block_size=256, trials=1, tsteps=10 | 105.0% | 105.0% | 1.14× | 4 |
| polybench_jacobi2d | block_size=256, trials=1, tsteps=50 | — | — | 0.41× | 0 |
| polybench_mvt | block_size=256, precision=double | 59.8% | 59.8% | 1.60× | 5 |
| polybench_mvt | block_size=256, precision=float | 61.6% | 61.6% | 1.61× | 5 |
| polybench_mvt | block_size=256, precision=half | 62.0% | 62.0% | 1.62× | 5 |
| polybench_syr2k | alpha=1.5, beta=1.2, block_size=256, inner_size=1024 | 3.5% | 3.5% | 0.85× | 3 |
| polybench_syr2k | alpha=1.5, beta=1.2, block_size=256, inner_size=2048 | 1.2% | 1.2% | 0.86× | 3 |
| polybench_syr2k | alpha=1.5, beta=1.2, block_size=256, inner_size=256 | 6.7% | 7.2% | 0.82× | 3 |
| polybench_syr2k | alpha=1.5, beta=1.2, block_size=256, inner_size=512 | 4.2% | 4.4% | 0.85× | 2 |
| rodinia_gaussian | block_size=256, verify=false | 28.7% | 40.2% | 0.33× | 1 |
| rodinia_gaussian | block_size=256, verify=skip | 29.4% | 41.6% | 0.34× | 1 |
| rodinia_gaussian | block_size=256, verify=true | 29.8% | 42.5% | 0.34× | 1 |
| rodinia_hotspot | block_size=16, num_iterations=10 | — | — | 0.16× | 0 |
| rodinia_hotspot | block_size=16, num_iterations=1 | 32.3% | 59.5% | 0.04× | 6 |
| rodinia_hotspot3d | amb_temp=100, block_size=8, num_iterations=1 | 7.4% | 8.8% | 0.58× | 6 |
| rodinia_hotspot3d | amb_temp=60, block_size=8, num_iterations=1 | 2.5% | 2.6% | 0.59× | 6 |
| rodinia_hotspot3d | amb_temp=80, block_size=8, num_iterations=1 | 1.3% | 1.3% | 0.58× | 5 |
| rodinia_lavamd | block_size=128, particles_per_box=100 | 9.6% | 10.6% | 0.85× | 2 |
| rodinia_lavamd | block_size=128, particles_per_box=150 | 9.4% | 10.3% | 0.86× | 2 |
| rodinia_lavamd | block_size=128, particles_per_box=200 | 9.9% | 11.0% | 0.86× | 1 |
| rodinia_lavamd | block_size=128, particles_per_box=50 | 10.2% | 11.4% | 0.85× | 1 |
| rodinia_lud | block_size=256, verify=false | 31.2% | 31.2% | 1.46× | 4 |
| rodinia_lud | block_size=256, verify=skip | 31.0% | 31.0% | 1.48× | 4 |
| rodinia_lud | block_size=256, verify=true | 29.4% | 29.4% | 1.48× | 4 |
| rodinia_srad | block_size=16, num_iterations=10 | 1.6% | 1.7% | 0.46× | 2 |
| shared_mem_latency | access_pattern=conflict, block_size=1024, num_blocks=1 | 65.7% | 223.2% | 0.24× | 7 |
| shared_mem_latency | access_pattern=conflict, block_size=256, num_blocks=1 | 65.6% | 222.2% | 0.24× | 7 |
| shared_mem_latency | access_pattern=conflict, block_size=32, num_blocks=1 | 34.3% | 53.9% | 0.59× | 6 |
| shared_mem_latency | access_pattern=conflict, block_size=64, num_blocks=1 | 49.2% | 103.3% | 0.41× | 6 |
| shared_mem_latency | access_pattern=no_conflict, block_size=1024, num_blocks=1 | 237.9% | 237.9% | 3.80× | 6 |
| shared_mem_latency | access_pattern=no_conflict, block_size=256, num_blocks=1 | 12.2% | 12.2% | 1.14× | 6 |
| shared_mem_latency | access_pattern=no_conflict, block_size=32, num_blocks=1 | 18.4% | 22.8% | 0.78× | 6 |
| shared_mem_latency | access_pattern=no_conflict, block_size=64, num_blocks=1 | 18.4% | 22.8% | 0.78× | 6 |
| tango_blackscholes | block_size=256, precision=double | 4.6% | 4.6% | 0.92× | 15 |
| tango_blackscholes | block_size=256, precision=float | 5.6% | 7.3% | 0.87× | 15 |
| tango_blackscholes | block_size=256, precision=half | 1.9% | 2.0% | 0.91× | 15 |

**Overall: MAPE 24.5% · sMAPE 431.7%** (mean of per-figure error over 93 figures)

## Global calibration

Cross-benchmark view, immune to per-benchmark fitting. **Rank correlation** (over every measured run, ranked together) grades whether the sim orders runs by cost like real HW (scale-invariant); **geo-σ** is the typical multiplicative error left after the single global constant `k` (so it can't be lowered by per-benchmark fudging). `k` is a reference, not a score.

- **Spearman ρ = 0.96** · **Kendall τ = 0.84** (≈92% of pairs ordered correctly)
- **geomean k = 0.71×** (single global constant, reference)
- **geo-σ = 2.10×** after that constant (ideal 1×) — shape 1.56× (within-benchmark) / level 2.21× (between-benchmark)

![benchmark ranking](figures/_global_rank.png)

![sim vs real absolute](figures/_global_scatter.png)



## altis_cfd

### block_size=256, precision=double

_offset +0.015 ms · MAPE 6.4% · sMAPE 7.0% (n=10, anchor excl.)_

![altis_cfd — block_size=256, precision=double](figures/altis_cfd_block_size-256_precision-double.png)

### block_size=256, precision=float

_offset +0.017 ms · MAPE 7.4% · sMAPE 7.9% (n=10, anchor excl.)_

![altis_cfd — block_size=256, precision=float](figures/altis_cfd_block_size-256_precision-float.png)

### block_size=256, precision=half

_offset +0.025 ms · MAPE 30.6% · sMAPE 30.6% (n=10, anchor excl.)_

![altis_cfd — block_size=256, precision=half](figures/altis_cfd_block_size-256_precision-half.png)


## altis_raytracing

### height=1024, spheres=256

_offset -0.020 ms · err = n/a (only the anchor matched)_

![altis_raytracing — height=1024, spheres=256](figures/altis_raytracing_height-1024_spheres-256.png)

### height=1024, spheres=64

_offset -0.005 ms · err = n/a (only the anchor matched)_

![altis_raytracing — height=1024, spheres=64](figures/altis_raytracing_height-1024_spheres-64.png)

### height=1024, spheres=8

_offset -0.003 ms · err = n/a (only the anchor matched)_

![altis_raytracing — height=1024, spheres=8](figures/altis_raytracing_height-1024_spheres-8.png)

### height=2048, spheres=256

_offset -0.020 ms · err = n/a (only the anchor matched)_

![altis_raytracing — height=2048, spheres=256](figures/altis_raytracing_height-2048_spheres-256.png)

### height=2048, spheres=64

_offset -0.007 ms · err = n/a (only the anchor matched)_

![altis_raytracing — height=2048, spheres=64](figures/altis_raytracing_height-2048_spheres-64.png)

### height=2048, spheres=8

_offset -0.003 ms · err = n/a (only the anchor matched)_

![altis_raytracing — height=2048, spheres=8](figures/altis_raytracing_height-2048_spheres-8.png)


## cache_latency

### cacheline_bytes=64, measure_laps=4, rng_seed=42

_offset +1.490 ms · MAPE 32.2% · sMAPE 32.2% (n=11, anchor excl.)_

![cache_latency — cacheline_bytes=64, measure_laps=4, rng_seed=42](figures/cache_latency_cacheline_bytes-64_measure_laps-4_rng_seed-42.png)


## empty_kernel

### block_size=1024

_offset +0.008 ms · MAPE 44.1% · sMAPE 44.1% (n=8, anchor excl.)_

![empty_kernel — block_size=1024](figures/empty_kernel_block_size-1024.png)

### block_size=256

_offset +0.008 ms · MAPE 1.4% · sMAPE 1.5% (n=8, anchor excl.)_

![empty_kernel — block_size=256](figures/empty_kernel_block_size-256.png)

### block_size=32

_offset +0.008 ms · MAPE 34.5% · sMAPE 100.4% (n=8, anchor excl.)_

![empty_kernel — block_size=32](figures/empty_kernel_block_size-32.png)

### block_size=64

_offset +0.008 ms · MAPE 34.6% · sMAPE 100.6% (n=8, anchor excl.)_

![empty_kernel — block_size=64](figures/empty_kernel_block_size-64.png)


## fp32_throughput

### num_blocks=1, threads_per_block=1024

_offset +0.003 ms · MAPE 17.5% · sMAPE 21.9% (n=12, anchor excl.)_

![fp32_throughput — num_blocks=1, threads_per_block=1024](figures/fp32_throughput_num_blocks-1_threads_per_block-1024.png)

### num_blocks=1, threads_per_block=1

_offset +0.003 ms · MAPE 10.4% · sMAPE 11.8% (n=12, anchor excl.)_

![fp32_throughput — num_blocks=1, threads_per_block=1](figures/fp32_throughput_num_blocks-1_threads_per_block-1.png)

### num_blocks=1, threads_per_block=256

_offset +0.004 ms · MAPE 9.1% · sMAPE 9.9% (n=12, anchor excl.)_

![fp32_throughput — num_blocks=1, threads_per_block=256](figures/fp32_throughput_num_blocks-1_threads_per_block-256.png)

### num_blocks=1, threads_per_block=32

_offset +0.004 ms · MAPE 10.7% · sMAPE 12.5% (n=12, anchor excl.)_

![fp32_throughput — num_blocks=1, threads_per_block=32](figures/fp32_throughput_num_blocks-1_threads_per_block-32.png)

### num_blocks=1, threads_per_block=64

_offset +0.004 ms · MAPE 11.6% · sMAPE 12.4% (n=12, anchor excl.)_

![fp32_throughput — num_blocks=1, threads_per_block=64](figures/fp32_throughput_num_blocks-1_threads_per_block-64.png)


## fp64_throughput

### num_blocks=1, threads_per_block=1024

_offset -0.002 ms · MAPE 55.7% · sMAPE 55.7% (n=12, anchor excl.)_

![fp64_throughput — num_blocks=1, threads_per_block=1024](figures/fp64_throughput_num_blocks-1_threads_per_block-1024.png)

### num_blocks=1, threads_per_block=1

_offset +0.002 ms · MAPE 6.3% · sMAPE 6.3% (n=12, anchor excl.)_

![fp64_throughput — num_blocks=1, threads_per_block=1](figures/fp64_throughput_num_blocks-1_threads_per_block-1.png)

### num_blocks=1, threads_per_block=256

_offset +0.002 ms · MAPE 5.8% · sMAPE 5.8% (n=12, anchor excl.)_

![fp64_throughput — num_blocks=1, threads_per_block=256](figures/fp64_throughput_num_blocks-1_threads_per_block-256.png)

### num_blocks=1, threads_per_block=32

_offset +0.004 ms · MAPE 10.0% · sMAPE 10.0% (n=12, anchor excl.)_

![fp64_throughput — num_blocks=1, threads_per_block=32](figures/fp64_throughput_num_blocks-1_threads_per_block-32.png)

### num_blocks=1, threads_per_block=64

_offset +0.002 ms · MAPE 5.5% · sMAPE 5.5% (n=12, anchor excl.)_

![fp64_throughput — num_blocks=1, threads_per_block=64](figures/fp64_throughput_num_blocks-1_threads_per_block-64.png)


## int32_throughput

### num_blocks=1, threads_per_block=1024

_offset +0.003 ms · MAPE 24.0% · sMAPE 37132.2% (n=12, anchor excl.)_

![int32_throughput — num_blocks=1, threads_per_block=1024](figures/int32_throughput_num_blocks-1_threads_per_block-1024.png)

### num_blocks=1, threads_per_block=1

_offset +0.003 ms · MAPE 31.8% · sMAPE 47.9% (n=12, anchor excl.)_

![int32_throughput — num_blocks=1, threads_per_block=1](figures/int32_throughput_num_blocks-1_threads_per_block-1.png)

### num_blocks=1, threads_per_block=256

_offset +0.003 ms · MAPE 29.7% · sMAPE 44.1% (n=12, anchor excl.)_

![int32_throughput — num_blocks=1, threads_per_block=256](figures/int32_throughput_num_blocks-1_threads_per_block-256.png)

### num_blocks=1, threads_per_block=32

_offset +0.003 ms · MAPE 29.9% · sMAPE 44.3% (n=12, anchor excl.)_

![int32_throughput — num_blocks=1, threads_per_block=32](figures/int32_throughput_num_blocks-1_threads_per_block-32.png)

### num_blocks=1, threads_per_block=64

_offset +0.003 ms · MAPE 29.5% · sMAPE 43.8% (n=12, anchor excl.)_

![int32_throughput — num_blocks=1, threads_per_block=64](figures/int32_throughput_num_blocks-1_threads_per_block-64.png)


## memory_bandwidth

### transfer_type=d2d, use_pinned_memory=true

_offset +0.004 ms · MAPE 6.9% · sMAPE 6.9% (n=1, anchor excl.)_

![memory_bandwidth — transfer_type=d2d, use_pinned_memory=true](figures/memory_bandwidth_transfer_type-d2d_use_pinned_memory-true.png)


## parboil_cutcp

### block_size=128, cutoff_radius=12, grid_spacing=0.25

_offset +0.145 ms · MAPE 39.6% · sMAPE 67.7% (n=7, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=12, grid_spacing=0.25](figures/parboil_cutcp_block_size-128_cutoff_radius-12_grid_spacing-0.25.png)

### block_size=128, cutoff_radius=12, grid_spacing=0.5

_offset +0.032 ms · MAPE 15.9% · sMAPE 19.2% (n=7, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=12, grid_spacing=0.5](figures/parboil_cutcp_block_size-128_cutoff_radius-12_grid_spacing-0.5.png)

### block_size=128, cutoff_radius=12, grid_spacing=1

_offset -0.014 ms · MAPE 5.5% · sMAPE 5.5% (n=7, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=12, grid_spacing=1](figures/parboil_cutcp_block_size-128_cutoff_radius-12_grid_spacing-1.png)

### block_size=128, cutoff_radius=12, grid_spacing=2

_offset -0.020 ms · MAPE 8.4% · sMAPE 8.4% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=12, grid_spacing=2](figures/parboil_cutcp_block_size-128_cutoff_radius-12_grid_spacing-2.png)

### block_size=128, cutoff_radius=20, grid_spacing=0.25

_offset +0.215 ms · MAPE 42.1% · sMAPE 75.8% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=20, grid_spacing=0.25](figures/parboil_cutcp_block_size-128_cutoff_radius-20_grid_spacing-0.25.png)

### block_size=128, cutoff_radius=20, grid_spacing=0.5

_offset +0.113 ms · MAPE 33.5% · sMAPE 51.6% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=20, grid_spacing=0.5](figures/parboil_cutcp_block_size-128_cutoff_radius-20_grid_spacing-0.5.png)

### block_size=128, cutoff_radius=20, grid_spacing=1

_offset +0.014 ms · MAPE 8.9% · sMAPE 10.0% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=20, grid_spacing=1](figures/parboil_cutcp_block_size-128_cutoff_radius-20_grid_spacing-1.png)

### block_size=128, cutoff_radius=20, grid_spacing=2

_offset -0.020 ms · MAPE 8.1% · sMAPE 8.1% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=20, grid_spacing=2](figures/parboil_cutcp_block_size-128_cutoff_radius-20_grid_spacing-2.png)

### block_size=128, cutoff_radius=6, grid_spacing=0.25

_offset +0.032 ms · MAPE 15.3% · sMAPE 18.4% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=6, grid_spacing=0.25](figures/parboil_cutcp_block_size-128_cutoff_radius-6_grid_spacing-0.25.png)

### block_size=128, cutoff_radius=6, grid_spacing=0.5

_offset -0.014 ms · MAPE 4.7% · sMAPE 4.7% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=6, grid_spacing=0.5](figures/parboil_cutcp_block_size-128_cutoff_radius-6_grid_spacing-0.5.png)

### block_size=128, cutoff_radius=6, grid_spacing=1

_offset -0.020 ms · MAPE 8.8% · sMAPE 8.8% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=6, grid_spacing=1](figures/parboil_cutcp_block_size-128_cutoff_radius-6_grid_spacing-1.png)

### block_size=128, cutoff_radius=6, grid_spacing=2

_offset -0.012 ms · MAPE 4.9% · sMAPE 4.9% (n=6, anchor excl.)_

![parboil_cutcp — block_size=128, cutoff_radius=6, grid_spacing=2](figures/parboil_cutcp_block_size-128_cutoff_radius-6_grid_spacing-2.png)


## parboil_lbm

### block_size=128, iterations=1, num_timesteps=10, tau=0.7

_offset +0.036 ms · MAPE 30.9% · sMAPE 30.9% (n=2, anchor excl.)_

![parboil_lbm — block_size=128, iterations=1, num_timesteps=10, tau=0.7](figures/parboil_lbm_block_size-128_iterations-1_num_timesteps-10_tau-0.7.png)


## parboil_sgemm

### block_size=256, precision=double

_offset -0.000 ms · MAPE 30.1% · sMAPE 30.1% (n=4, anchor excl.)_

![parboil_sgemm — block_size=256, precision=double](figures/parboil_sgemm_block_size-256_precision-double.png)

### block_size=256, precision=float

_offset -0.001 ms · MAPE 30.8% · sMAPE 30.8% (n=4, anchor excl.)_

![parboil_sgemm — block_size=256, precision=float](figures/parboil_sgemm_block_size-256_precision-float.png)

### block_size=256, precision=half

_offset -0.000 ms · MAPE 29.9% · sMAPE 29.9% (n=4, anchor excl.)_

![parboil_sgemm — block_size=256, precision=half](figures/parboil_sgemm_block_size-256_precision-half.png)


## polybench_2dconv

### block_size=256, precision=double

_offset +0.003 ms · MAPE 10.3% · sMAPE 10.3% (n=6, anchor excl.)_

![polybench_2dconv — block_size=256, precision=double](figures/polybench_2dconv_block_size-256_precision-double.png)

### block_size=256, precision=float

_offset +0.003 ms · MAPE 11.9% · sMAPE 13.2% (n=6, anchor excl.)_

![polybench_2dconv — block_size=256, precision=float](figures/polybench_2dconv_block_size-256_precision-float.png)

### block_size=256, precision=half

_offset +0.004 ms · MAPE 16.8% · sMAPE 16.8% (n=6, anchor excl.)_

![polybench_2dconv — block_size=256, precision=half](figures/polybench_2dconv_block_size-256_precision-half.png)


## polybench_2mm

### alpha=1.5, beta=1.2, block_size=256, precision=double

_offset +0.002 ms · MAPE 17.7% · sMAPE 17.7% (n=5, anchor excl.)_

![polybench_2mm — alpha=1.5, beta=1.2, block_size=256, precision=double](figures/polybench_2mm_alpha-1.5_beta-1.2_block_size-256_precision-double.png)

### alpha=1.5, beta=1.2, block_size=256, precision=float

_offset +0.003 ms · MAPE 23.1% · sMAPE 23.1% (n=5, anchor excl.)_

![polybench_2mm — alpha=1.5, beta=1.2, block_size=256, precision=float](figures/polybench_2mm_alpha-1.5_beta-1.2_block_size-256_precision-float.png)

### alpha=1.5, beta=1.2, block_size=256, precision=half

_offset +0.002 ms · MAPE 17.0% · sMAPE 17.0% (n=5, anchor excl.)_

![polybench_2mm — alpha=1.5, beta=1.2, block_size=256, precision=half](figures/polybench_2mm_alpha-1.5_beta-1.2_block_size-256_precision-half.png)


## polybench_3dconv

### block_size=8, filter_size=3

_offset +0.001 ms · MAPE 4.1% · sMAPE 4.1% (n=4, anchor excl.)_

![polybench_3dconv — block_size=8, filter_size=3](figures/polybench_3dconv_block_size-8_filter_size-3.png)

### block_size=8, filter_size=5

_offset +0.003 ms · MAPE 8.1% · sMAPE 8.1% (n=4, anchor excl.)_

![polybench_3dconv — block_size=8, filter_size=5](figures/polybench_3dconv_block_size-8_filter_size-5.png)

### block_size=8, filter_size=7

_offset +0.007 ms · MAPE 14.7% · sMAPE 14.7% (n=3, anchor excl.)_

![polybench_3dconv — block_size=8, filter_size=7](figures/polybench_3dconv_block_size-8_filter_size-7.png)


## polybench_3mm

### block_size=256, precision=double

_offset -0.015 ms · MAPE 35.0% · sMAPE 35.0% (n=4, anchor excl.)_

![polybench_3mm — block_size=256, precision=double](figures/polybench_3mm_block_size-256_precision-double.png)

### block_size=256, precision=float

_offset -0.015 ms · MAPE 35.8% · sMAPE 35.8% (n=4, anchor excl.)_

![polybench_3mm — block_size=256, precision=float](figures/polybench_3mm_block_size-256_precision-float.png)

### block_size=256, precision=half

_offset -0.012 ms · MAPE 39.8% · sMAPE 39.8% (n=4, anchor excl.)_

![polybench_3mm — block_size=256, precision=half](figures/polybench_3mm_block_size-256_precision-half.png)


## polybench_correlation

### block_size=256, precision=double

_offset +0.001 ms · MAPE 5.3% · sMAPE 5.3% (n=6, anchor excl.)_

![polybench_correlation — block_size=256, precision=double](figures/polybench_correlation_block_size-256_precision-double.png)

### block_size=256, precision=float

_offset +0.002 ms · MAPE 5.9% · sMAPE 5.9% (n=6, anchor excl.)_

![polybench_correlation — block_size=256, precision=float](figures/polybench_correlation_block_size-256_precision-float.png)

### block_size=256, precision=half

_offset +0.001 ms · MAPE 6.8% · sMAPE 7.0% (n=6, anchor excl.)_

![polybench_correlation — block_size=256, precision=half](figures/polybench_correlation_block_size-256_precision-half.png)


## polybench_fdtd2d

### block_size=256, tmax=10

_offset +0.166 ms · MAPE 5.0% · sMAPE 5.3% (n=4, anchor excl.)_

![polybench_fdtd2d — block_size=256, tmax=10](figures/polybench_fdtd2d_block_size-256_tmax-10.png)

### block_size=256, tmax=50

_offset +0.824 ms · err = n/a (only the anchor matched)_

![polybench_fdtd2d — block_size=256, tmax=50](figures/polybench_fdtd2d_block_size-256_tmax-50.png)


## polybench_gemm

### block_size=256, precision=double

_offset +0.001 ms · MAPE 53.8% · sMAPE 53.8% (n=4, anchor excl.)_

![polybench_gemm — block_size=256, precision=double](figures/polybench_gemm_block_size-256_precision-double.png)

### block_size=256, precision=float

_offset -0.000 ms · MAPE 41.9% · sMAPE 41.9% (n=4, anchor excl.)_

![polybench_gemm — block_size=256, precision=float](figures/polybench_gemm_block_size-256_precision-float.png)

### block_size=256, precision=half

_offset -0.001 ms · MAPE 41.1% · sMAPE 41.1% (n=4, anchor excl.)_

![polybench_gemm — block_size=256, precision=half](figures/polybench_gemm_block_size-256_precision-half.png)


## polybench_jacobi2d

### block_size=256, trials=1, tsteps=10

_offset +0.015 ms · MAPE 105.0% · sMAPE 105.0% (n=4, anchor excl.)_

![polybench_jacobi2d — block_size=256, trials=1, tsteps=10](figures/polybench_jacobi2d_block_size-256_trials-1_tsteps-10.png)

### block_size=256, trials=1, tsteps=50

_offset +0.074 ms · err = n/a (only the anchor matched)_

![polybench_jacobi2d — block_size=256, trials=1, tsteps=50](figures/polybench_jacobi2d_block_size-256_trials-1_tsteps-50.png)


## polybench_mvt

### block_size=256, precision=double

_offset -0.007 ms · MAPE 59.8% · sMAPE 59.8% (n=5, anchor excl.)_

![polybench_mvt — block_size=256, precision=double](figures/polybench_mvt_block_size-256_precision-double.png)

### block_size=256, precision=float

_offset -0.007 ms · MAPE 61.6% · sMAPE 61.6% (n=5, anchor excl.)_

![polybench_mvt — block_size=256, precision=float](figures/polybench_mvt_block_size-256_precision-float.png)

### block_size=256, precision=half

_offset -0.007 ms · MAPE 62.0% · sMAPE 62.0% (n=5, anchor excl.)_

![polybench_mvt — block_size=256, precision=half](figures/polybench_mvt_block_size-256_precision-half.png)


## polybench_syr2k

### alpha=1.5, beta=1.2, block_size=256, inner_size=1024

_offset +0.014 ms · MAPE 3.5% · sMAPE 3.5% (n=3, anchor excl.)_

![polybench_syr2k — alpha=1.5, beta=1.2, block_size=256, inner_size=1024](figures/polybench_syr2k_alpha-1.5_beta-1.2_block_size-256_inner_size-1024.png)

### alpha=1.5, beta=1.2, block_size=256, inner_size=2048

_offset +0.019 ms · MAPE 1.2% · sMAPE 1.2% (n=3, anchor excl.)_

![polybench_syr2k — alpha=1.5, beta=1.2, block_size=256, inner_size=2048](figures/polybench_syr2k_alpha-1.5_beta-1.2_block_size-256_inner_size-2048.png)

### alpha=1.5, beta=1.2, block_size=256, inner_size=256

_offset +0.003 ms · MAPE 6.7% · sMAPE 7.2% (n=3, anchor excl.)_

![polybench_syr2k — alpha=1.5, beta=1.2, block_size=256, inner_size=256](figures/polybench_syr2k_alpha-1.5_beta-1.2_block_size-256_inner_size-256.png)

### alpha=1.5, beta=1.2, block_size=256, inner_size=512

_offset +0.005 ms · MAPE 4.2% · sMAPE 4.4% (n=2, anchor excl.)_

![polybench_syr2k — alpha=1.5, beta=1.2, block_size=256, inner_size=512](figures/polybench_syr2k_alpha-1.5_beta-1.2_block_size-256_inner_size-512.png)


## rodinia_gaussian

### block_size=256, verify=false

_offset +0.524 ms · MAPE 28.7% · sMAPE 40.2% (n=1, anchor excl.)_

![rodinia_gaussian — block_size=256, verify=false](figures/rodinia_gaussian_block_size-256_verify-false.png)

### block_size=256, verify=skip

_offset +0.509 ms · MAPE 29.4% · sMAPE 41.6% (n=1, anchor excl.)_

![rodinia_gaussian — block_size=256, verify=skip](figures/rodinia_gaussian_block_size-256_verify-skip.png)

### block_size=256, verify=true

_offset +0.502 ms · MAPE 29.8% · sMAPE 42.5% (n=1, anchor excl.)_

![rodinia_gaussian — block_size=256, verify=true](figures/rodinia_gaussian_block_size-256_verify-true.png)


## rodinia_hotspot

### block_size=16, num_iterations=10

_offset +0.047 ms · err = n/a (only the anchor matched)_

![rodinia_hotspot — block_size=16, num_iterations=10](figures/rodinia_hotspot_block_size-16_num_iterations-10.png)

### block_size=16, num_iterations=1

_offset +0.042 ms · MAPE 32.3% · sMAPE 59.5% (n=6, anchor excl.)_

![rodinia_hotspot — block_size=16, num_iterations=1](figures/rodinia_hotspot_block_size-16_num_iterations-1.png)


## rodinia_hotspot3d

### amb_temp=100, block_size=8, num_iterations=1

_offset +0.002 ms · MAPE 7.4% · sMAPE 8.8% (n=6, anchor excl.)_

![rodinia_hotspot3d — amb_temp=100, block_size=8, num_iterations=1](figures/rodinia_hotspot3d_amb_temp-100_block_size-8_num_iterations-1.png)

### amb_temp=60, block_size=8, num_iterations=1

_offset +0.002 ms · MAPE 2.5% · sMAPE 2.6% (n=6, anchor excl.)_

![rodinia_hotspot3d — amb_temp=60, block_size=8, num_iterations=1](figures/rodinia_hotspot3d_amb_temp-60_block_size-8_num_iterations-1.png)

### amb_temp=80, block_size=8, num_iterations=1

_offset +0.002 ms · MAPE 1.3% · sMAPE 1.3% (n=5, anchor excl.)_

![rodinia_hotspot3d — amb_temp=80, block_size=8, num_iterations=1](figures/rodinia_hotspot3d_amb_temp-80_block_size-8_num_iterations-1.png)


## rodinia_lavamd

### block_size=128, particles_per_box=100

_offset +0.019 ms · MAPE 9.6% · sMAPE 10.6% (n=2, anchor excl.)_

![rodinia_lavamd — block_size=128, particles_per_box=100](figures/rodinia_lavamd_block_size-128_particles_per_box-100.png)

### block_size=128, particles_per_box=150

_offset +0.051 ms · MAPE 9.4% · sMAPE 10.3% (n=2, anchor excl.)_

![rodinia_lavamd — block_size=128, particles_per_box=150](figures/rodinia_lavamd_block_size-128_particles_per_box-150.png)

### block_size=128, particles_per_box=200

_offset +0.062 ms · MAPE 9.9% · sMAPE 11.0% (n=1, anchor excl.)_

![rodinia_lavamd — block_size=128, particles_per_box=200](figures/rodinia_lavamd_block_size-128_particles_per_box-200.png)

### block_size=128, particles_per_box=50

_offset +0.010 ms · MAPE 10.2% · sMAPE 11.4% (n=1, anchor excl.)_

![rodinia_lavamd — block_size=128, particles_per_box=50](figures/rodinia_lavamd_block_size-128_particles_per_box-50.png)


## rodinia_lud

### block_size=256, verify=false

_offset -0.035 ms · MAPE 31.2% · sMAPE 31.2% (n=4, anchor excl.)_

![rodinia_lud — block_size=256, verify=false](figures/rodinia_lud_block_size-256_verify-false.png)

### block_size=256, verify=skip

_offset -0.037 ms · MAPE 31.0% · sMAPE 31.0% (n=4, anchor excl.)_

![rodinia_lud — block_size=256, verify=skip](figures/rodinia_lud_block_size-256_verify-skip.png)

### block_size=256, verify=true

_offset -0.039 ms · MAPE 29.4% · sMAPE 29.4% (n=4, anchor excl.)_

![rodinia_lud — block_size=256, verify=true](figures/rodinia_lud_block_size-256_verify-true.png)


## rodinia_srad

### block_size=16, num_iterations=10

_offset +0.066 ms · MAPE 1.6% · sMAPE 1.7% (n=2, anchor excl.)_

![rodinia_srad — block_size=16, num_iterations=10](figures/rodinia_srad_block_size-16_num_iterations-10.png)


## shared_mem_latency

### access_pattern=conflict, block_size=1024, num_blocks=1

_offset +0.574 ms · MAPE 65.7% · sMAPE 223.2% (n=7, anchor excl.)_

![shared_mem_latency — access_pattern=conflict, block_size=1024, num_blocks=1](figures/shared_mem_latency_access_pattern-conflict_block_size-1024_num_blocks-1.png)

### access_pattern=conflict, block_size=256, num_blocks=1

_offset +0.575 ms · MAPE 65.6% · sMAPE 222.2% (n=7, anchor excl.)_

![shared_mem_latency — access_pattern=conflict, block_size=256, num_blocks=1](figures/shared_mem_latency_access_pattern-conflict_block_size-256_num_blocks-1.png)

### access_pattern=conflict, block_size=32, num_blocks=1

_offset +0.649 ms · MAPE 34.3% · sMAPE 53.9% (n=6, anchor excl.)_

![shared_mem_latency — access_pattern=conflict, block_size=32, num_blocks=1](figures/shared_mem_latency_access_pattern-conflict_block_size-32_num_blocks-1.png)

### access_pattern=conflict, block_size=64, num_blocks=1

_offset +0.666 ms · MAPE 49.2% · sMAPE 103.3% (n=6, anchor excl.)_

![shared_mem_latency — access_pattern=conflict, block_size=64, num_blocks=1](figures/shared_mem_latency_access_pattern-conflict_block_size-64_num_blocks-1.png)

### access_pattern=no_conflict, block_size=1024, num_blocks=1

_offset -0.130 ms · MAPE 237.9% · sMAPE 237.9% (n=6, anchor excl.)_

![shared_mem_latency — access_pattern=no_conflict, block_size=1024, num_blocks=1](figures/shared_mem_latency_access_pattern-no_conflict_block_size-1024_num_blocks-1.png)

### access_pattern=no_conflict, block_size=256, num_blocks=1

_offset -0.020 ms · MAPE 12.2% · sMAPE 12.2% (n=6, anchor excl.)_

![shared_mem_latency — access_pattern=no_conflict, block_size=256, num_blocks=1](figures/shared_mem_latency_access_pattern-no_conflict_block_size-256_num_blocks-1.png)

### access_pattern=no_conflict, block_size=32, num_blocks=1

_offset +0.264 ms · MAPE 18.4% · sMAPE 22.8% (n=6, anchor excl.)_

![shared_mem_latency — access_pattern=no_conflict, block_size=32, num_blocks=1](figures/shared_mem_latency_access_pattern-no_conflict_block_size-32_num_blocks-1.png)

### access_pattern=no_conflict, block_size=64, num_blocks=1

_offset +0.134 ms · MAPE 18.4% · sMAPE 22.8% (n=6, anchor excl.)_

![shared_mem_latency — access_pattern=no_conflict, block_size=64, num_blocks=1](figures/shared_mem_latency_access_pattern-no_conflict_block_size-64_num_blocks-1.png)


## tango_blackscholes

### block_size=256, precision=double

_offset +0.001 ms · MAPE 4.6% · sMAPE 4.6% (n=15, anchor excl.)_

![tango_blackscholes — block_size=256, precision=double](figures/tango_blackscholes_block_size-256_precision-double.png)

### block_size=256, precision=float

_offset +0.001 ms · MAPE 5.6% · sMAPE 7.3% (n=15, anchor excl.)_

![tango_blackscholes — block_size=256, precision=float](figures/tango_blackscholes_block_size-256_precision-float.png)

### block_size=256, precision=half

_offset +0.000 ms · MAPE 1.9% · sMAPE 2.0% (n=15, anchor excl.)_

![tango_blackscholes — block_size=256, precision=half](figures/tango_blackscholes_block_size-256_precision-half.png)

