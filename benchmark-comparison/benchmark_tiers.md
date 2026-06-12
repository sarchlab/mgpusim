# Benchmark Tiering System

Benchmarks are classified into tiers based on their simulated kernel execution
time (`sim_ms` from `comparison_ci.csv`). Faster-simulating benchmarks are in
lower tiers and are useful for rapid iteration during development.

## Tier Definitions

| Tier | Sim Time (max sim_ms) | Use Case |
|------|-----------------------|----------|
| **Tier 1 — Fast** | < 0.05 ms | Run first for rapid iteration. Quick smoke tests. |
| **Tier 2 — Medium** | 0.05 – 0.3 ms | Run after Tier 1 passes. More complex workloads. |
| **Tier 3 — Slow** | > 0.3 ms | Full validation. Run in CI or before releases. |

## Benchmark Classification

### Tier 1 — Fast (< 0.05 ms max sim_ms)

| Benchmark | Max sim_ms | Sizes with data |
|-----------|-----------|-----------------|
| 3dconvolution | 0.008 | 2 |
| reduction | 0.011 | 8 |
| 2dconvolution | 0.017 | 12 |
| spmv_csr | 0.023 | 9 |
| bs | 0.024 | 11 |
| nn | 0.025 | 11 |
| gemm | 0.032 | 4 |
| scan | 0.036 | 11 |
| computeq | 0.039 | 5 |
| ep | 0.039 | 12 |
| triad | 0.043 | 12 |
| 2mm | 0.044 | 3 |
| hotspot | 0.047 | 9 |
| sgemm_tiled | 0.049 | 7 |
| dwt2d | 0.049 | 7 |

### Tier 2 — Medium (0.05 – 0.3 ms max sim_ms)

| Benchmark | Max sim_ms | Sizes with data |
|-----------|-----------|-----------------|
| cutoff_potential | 0.057 | 2 |
| syrk | 0.058 | 3 |
| 3mm | 0.062 | 3 |
| syr2k | 0.062 | 3 |
| correlation | 0.069 | 4 |
| stencil_kernel | 0.073 | 8 |
| covariance | 0.084 | 5 |
| srad | 0.090 | 4 |
| fdtd2d | 0.175 | 5 |
| hotspot3d | 0.201 | 6 |
| mvt | 0.236 | 6 |
| atax | 0.272 | 6 |
| bicg | 0.279 | 6 |
| computesad | 0.282 | 4 |

### Tier 3 — Slow (> 0.3 ms max sim_ms)

| Benchmark | Max sim_ms | Sizes with data |
|-----------|-----------|-----------------|
| gesummv | 0.314 | 5 |
| lavamd | 0.382 | 3 |
| pathfinder | 0.437 | 6 |
| sort | 0.505 | 6 |
| layerforward | 3.115 | 8 |

## Notes

- Tier thresholds are based on the maximum `sim_ms` across all problem sizes
  for each benchmark.
- `sim_ms` is the simulated kernel execution time on the GPU, not the wall-clock
  time to run the simulation. Actual simulation wall-clock time depends on host
  machine and simulator overhead.
- As more benchmark sizes are added, classifications may shift.
- Use `run_tier1.sh` for quick local testing with Tier 1 benchmarks only.
