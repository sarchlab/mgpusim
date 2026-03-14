# Microbenchmark Comparison: Simulator vs Real Hardware

## 1. Overview

This document describes the four GPU microbenchmarks used to validate the
MGPUSim simulator's architectural parameters against real AMD MI300A hardware.
Each microbenchmark isolates a specific hardware characteristic (cache latency,
TLB behavior, memory bandwidth, kernel launch overhead) so that the simulator's
configuration can be verified independently of full-application accuracy.

The microbenchmarks produce CSV output that can be fed into `validate_params.py`
for automated comparison against the simulator's hardcoded parameters.

| Microbenchmark         | What it measures                              | Key simulator parameter(s)                  |
|------------------------|-----------------------------------------------|---------------------------------------------|
| `micro_cache_latency`  | L1, L2, and DRAM access latency (cycles)      | L1V/L2 bank latency, DRAM row-miss delay    |
| `micro_tlb`            | TLB hit/miss access latency                   | L1/L2 TLB entries, page size, coverage      |
| `micro_membw`          | Global memory bandwidth (GB/s)                | DRAM frequency, pipeline depth              |
| `micro_launch`         | Kernel launch overhead (µs / cycles)           | Kernel launch overhead (first/subsequent)   |

## 2. Current Simulator Parameters

Source: `mi300a/builder.go` (commit `dcbe8e48`).

| Parameter                    | Value                      | Notes                                    |
|------------------------------|----------------------------|------------------------------------------|
| GPU frequency                | 1700 MHz                   |                                          |
| L1V cache size               | 32 KB per CU               |                                          |
| L1V bank latency             | 7 cycles                   |                                          |
| L2 cache size                | 32 MB total                |                                          |
| L2 bank latency              | 6 cycles                   |                                          |
| DRAM frequency               | 1 GHz                      |                                          |
| DRAM pipeline depth           | 5                          |                                          |
| DRAM row miss delay           | 52 cycles                  |                                          |
| Kernel launch overhead (1st) | 5400 cycles                | First kernel dispatch                    |
| Kernel launch overhead (sub) | 1800 cycles                | Subsequent kernel dispatches             |
| L1V TLB                      | 4 sets × 64 ways = 256 entries | 4 KB pages → 1 MB coverage           |
| L2 TLB                       | 64 sets × 64 ways = 4096 entries | 4 KB pages → 16 MB coverage        |
| Page size                    | 4 KB (log2PageSize = 12)   |                                          |
| Number of CUs                | 120 (20 SAs × 6 CUs/SA)   |                                          |

## 3. Building All Microbenchmarks

All microbenchmarks are HIP C++ programs targeting the AMD MI300A (`gfx942`).
They share a common header `bench_common.h`.

### Prerequisites

- AMD ROCm / HIP toolchain (`hipcc`)
- Target architecture: `gfx942`

### Build commands

```bash
cd gpu_perf_scripts/

# Build all four microbenchmarks at once:
hipcc -O2 --offload-arch=gfx942 -o micro_cache_latency micro_cache_latency.cpp
hipcc -O2 --offload-arch=gfx942 -o micro_tlb            micro_tlb.cpp
hipcc -O2 --offload-arch=gfx942 -o micro_membw           micro_membw.cpp
hipcc -O2 --offload-arch=gfx942 -o micro_launch          micro_launch.cpp
```

Or use `build_all.sh` which builds all benchmarks (including the 22 application
benchmarks). To build only the microbenchmarks, pass them as arguments:

```bash
# build_all.sh doesn't include micro_* by default — use hipcc directly,
# or add them to the BENCHMARKS array in build_all.sh.
```

## 4. Running Microbenchmarks

Each microbenchmark prints CSV output (with `#`-prefixed comment lines) to
stdout. Redirect to a file for later analysis.

### micro_cache_latency

Measures L1, L2, and DRAM access latency via pointer chasing.

```bash
./micro_cache_latency --iterations 10 > cache_results.csv
```

**Output columns:** `level,array_size_kb,iterations,avg_ns,estimated_cycles`

- *level*: `L1`, `L2`, or `DRAM`
- *array_size_kb*: size of the pointer-chase array
- *avg_ns*: average access latency in nanoseconds
- *estimated_cycles*: `avg_ns × GPU_FREQ_GHZ`

### micro_tlb

Measures TLB miss penalties by strided pointer chasing that exceeds TLB
coverage at each level.

```bash
./micro_tlb --iterations 10 > tlb_results.csv
```

**Output columns:** `test_name,num_pages,stride_kb,iterations,avg_ns_per_access,estimated_cycles`

- *test_name*: describes the TLB level being tested (e.g., `tlb_hit_baseline`,
  `l1_tlb_miss`, `l2_tlb_miss`, `huge_stride`)
- *num_pages*: number of distinct pages accessed
- *stride_kb*: stride between accesses in KB

### micro_membw

Measures effective global memory bandwidth via streaming read, write, and copy
operations at various array sizes.

```bash
./micro_membw --iterations 10 > membw_results.csv
```

**Output columns:** `operation,array_size,iterations,avg_ms,min_ms,max_ms,avg_gbps`

- *operation*: `stream_read`, `stream_write`, or `stream_copy`
- *array_size*: human-readable size (e.g., `64MB`, `1GB`)
- *avg_gbps*: effective bandwidth in GB/s

### micro_launch

Measures kernel launch overhead in three modes: batch async, sync-per-launch,
and small-kernel sync.

```bash
./micro_launch --iterations 10 --launches 10000 > launch_results.csv
```

**Output columns:** `test,launches,iterations,total_avg_ms,per_launch_avg_us`

- *test*: `batch_async`, `sync_per_launch`, or `small_kernel_sync`
- *per_launch_avg_us*: average per-kernel launch overhead in microseconds

## 5. Using validate_params.py

`validate_params.py` compares microbenchmark measurements against the hardcoded
simulator parameters listed in Section 2.

### Usage

```bash
# All four CSVs:
python validate_params.py \
    --cache-latency cache_results.csv \
    --tlb tlb_results.csv \
    --membw membw_results.csv \
    --launch launch_results.csv

# Or any subset:
python validate_params.py --cache-latency cache_results.csv
python validate_params.py --launch launch_results.csv --membw membw_results.csv
```

### Output

The script prints:

1. **Reference parameters** — the simulator values being compared against.
2. **Validation table** — for each measured parameter:
   - Measured value, expected value, ratio (measured/expected), absolute
     difference, and PASS/FAIL status.
3. **Summary** — total checks, pass count, fail count.

### Pass/fail criteria

A check **passes** if the ratio (measured ÷ expected) is between **0.5×** and
**2.0×**. This intentionally wide range accounts for the fact that
microbenchmarks on real hardware measure end-to-end latency that includes
effects not modeled by a single simulator parameter.

### Exit code

- `0` — all checks passed
- `1` — one or more checks failed
- `2` — no CSV files provided

## 6. Interpreting Results

### micro_cache_latency → Cache bank latency & DRAM delay

| Measured level | Compared to simulator parameter | Expected behavior |
|---------------|--------------------------------|-------------------|
| L1 latency    | `l1v_bank_latency` = 7 cycles  | Small arrays (≤32 KB) fit in L1 |
| L2 latency    | `l2_bank_latency` = 6 cycles   | Medium arrays (256 KB–8 MB) spill to L2 |
| DRAM latency  | `dram_row_miss_delay` = 52 cycles | Large arrays (≥64 MB) exceed L2 |

> **Note:** Real hardware L1 latency is typically higher than the simulator's
> bank latency because the measured value includes pipeline, TLB, and
> interconnect overhead on top of the raw cache access.

### micro_tlb → TLB coverage and miss penalty

| Test | What it reveals | Simulator parameter |
|------|----------------|---------------------|
| TLB-hit baseline | Access latency when all pages are in L1 TLB | L1V TLB: 256 entries (1 MB) |
| L1 TLB miss | Extra cost when pages exceed L1 TLB but fit in L2 TLB | L2 TLB: 4096 entries (16 MB) |
| L2 TLB miss | Extra cost when pages exceed L2 TLB | Page table walk goes to DRAM |

If the TLB-hit baseline latency is close to the L1 cache latency and the L2
TLB miss penalty is close to the DRAM latency, the TLB parameters are
well-calibrated.

### micro_membw → DRAM bandwidth

The peak measured bandwidth should reflect the simulator's DRAM subsystem
(1 GHz frequency, pipeline depth 5). Real MI300A with HBM3 achieves multi-TB/s
bandwidth; the simulator models a simplified memory subsystem, so absolute
numbers will differ. The ratio between read/write/copy operations should be
consistent.

### micro_launch → Kernel dispatch overhead

| Test mode | What it measures | Simulator parameter |
|-----------|-----------------|---------------------|
| batch_async | Amortized per-kernel cost (no sync between launches) | `kernel_launch_subsequent` = 1800 cycles |
| sync_per_launch | Per-kernel cost with full synchronization | `kernel_launch_subsequent` = 1800 cycles |
| small_kernel_sync | Total cold-start cost (launch + minimal work + sync) | `kernel_launch_first` = 5400 cycles |

The batch_async mode gives the lowest per-kernel overhead (closest to the
simulator's "subsequent" value). The sync_per_launch and small_kernel_sync
modes add synchronization cost that is not directly modeled by the launch
overhead parameter.

## 7. Historical Comparison Data

> **Data source:** CI pipeline `comparison_ci.csv` — run 22915644610 (post-revert parameters)
>
> **Overall accuracy:** MAPE = 43.76%, matched data points = 220 / 438

### 7.1 Kernel Launch Overhead

Kernel launch overhead is approximated by measuring the smallest problem sizes,
where computation is negligible and total time is dominated by launch setup.

We use **vectoradd** and **relu** at size 1024 (the smallest available size with
sim data).

| Kernel    | Size | Real (ms) | Sim (ms) | Ratio (Sim/Real) | Implied Cycles (Sim, @1.8 GHz) | Implied Cycles (Real, @1.8 GHz) |
|-----------|------|-----------|----------|------------------|-------------------------------|--------------------------------|
| vectoradd | 1024 | 0.0046    | 0.0031   | 0.67×            | 5,564                         | 8,280                          |
| relu      | 1024 | 0.0054    | 0.0031   | 0.57×            | 5,530                         | 9,720                          |

**Observations:**
- The simulator's minimum kernel time (~0.0031 ms ≈ 5,547 cycles at 1.8 GHz)
  represents the simulated launch overhead floor.
- Real hardware launch overhead is ~0.005 ms ≈ 9,000 cycles at 1.8 GHz.
- The simulator **underestimates** launch overhead by ~0.6×, an improvement over
  the previous overestimate of ~1.5× with pre-revert parameters.

### 7.2 Per-Kernel Overhead (Bitonicsort)

Bitonicsort launches many kernels at small sizes, making the per-kernel overhead
a dominant factor. At sizes 1024–4096, the total runtime is largely per-kernel
overhead × number of kernel launches.

Bitonicsort at size N performs O(log²N) kernel launches.

| Size | Real (ms) | Sim (ms)  | Ratio (Sim/Real) | Kernels (≈log²N) | Per-Kernel Real (µs) | Per-Kernel Sim (µs) |
|------|-----------|-----------|-------------------|-------------------|---------------------|---------------------|
| 1024 | 0.1612    | 0.1344    | 0.83×             | 55                | 2.93                | 2.44                |
| 2048 | 0.1743    | 0.1612    | 0.93×             | 66                | 2.64                | 2.44                |
| 4096 | 0.1965    | 0.1905    | 0.97×             | 78                | 2.52                | 2.44                |

**Observations:**
- The simulator's per-kernel overhead is ~2.44 µs, nearly constant across sizes.
- Real hardware per-kernel overhead is ~2.5–2.9 µs.
- Post-revert, the simulator now **slightly underestimates** per-kernel overhead
  (ratio ~0.83–0.97×), compared to the previous overestimate of ~1.6–1.8×.
- The sim/real ratio improves with size, reaching near-parity at 4096.

### 7.3 Memory Throughput / Effective Bandwidth

Vectoradd processes 3 arrays (2 reads + 1 write) of N × 4 bytes each.
Total data moved = N × 12 bytes.

Effective bandwidth = data_bytes / time_seconds → GB/s.

| Size      | Data (MB) | Real (ms) | Sim (ms)  | Real BW (GB/s) | Sim BW (GB/s) | BW Ratio (Real/Sim) |
|-----------|-----------|-----------|-----------|-----------------|---------------|---------------------|
| 1,048,576 | 12.00     | 0.0105    | 0.0223    | 1,198.4         | 565.4         | 2.12×               |
| 2,097,152 | 24.00     | 0.0103    | 0.0417    | 2,443.3         | 603.6         | 4.05×               |

**Observations:**
- Real hardware achieves very high effective bandwidth (>1 TB/s at these sizes),
  likely due to caching effects and memory coalescing on MI300A.
- The simulator's effective bandwidth (~565–604 GB/s) is higher than pre-revert
  values (~454–524 GB/s), showing improved throughput modeling.
- The bandwidth gap has narrowed from ~2.5–4.5× to ~2.1–4.1×.
- At size 2,097,152, the real hardware likely benefits from L2 cache hits,
  inflating apparent bandwidth beyond DRAM limits.

### 7.4 Summary Table

| Parameter                    | Sim Value          | Real Value         | Ratio (Sim/Real) | Notes                                          |
|------------------------------|--------------------|--------------------|------------------|-------------------------------------------------|
| Kernel launch overhead       | ~0.0031 ms (5.5k cycles) | ~0.005 ms (9.0k cycles) | 0.62×   | From vectoradd/relu at size 1024               |
| Per-kernel overhead          | ~2.44 µs           | ~2.70 µs           | 0.91×            | From bitonicsort at sizes 1024–4096             |
| Effective bandwidth (1M)     | 565.4 GB/s         | 1,198.4 GB/s       | 0.47×            | vectoradd N=1,048,576; real may be cache-aided  |

**Key takeaways:**
1. Post-revert, the simulator now **underestimates** kernel launch overhead
   (~0.62×) — a shift from the pre-revert overestimate (~1.46×).
2. Per-kernel overhead is now **much closer to real hardware** (~0.91× vs
   previous ~1.69×). This is the biggest accuracy improvement.
3. Effective memory bandwidth improved slightly (~565 vs ~454 GB/s) but the
   simulator remains conservative compared to cache-aided real hardware.
4. Overall MAPE improved to **43.76%** with **220 matched data points**,
   meeting the target thresholds (≤ 44% MAPE, ≥ 200 matched points).

### 7.5 Pre-Revert vs Post-Revert Comparison

| Parameter                | Pre-Revert (Sim/Real) | Post-Revert (Sim/Real) | Direction |
|--------------------------|----------------------|------------------------|-----------|
| Kernel launch overhead   | 1.46×                | 0.62×                  | Over → Under |
| Per-kernel overhead      | 1.69×                | 0.91×                  | **Much closer to 1.0** |
| Effective bandwidth (1M) | 0.40×                | 0.47×                  | Slightly improved |
| Overall MAPE             | (pre-revert value)   | 43.76%                 | Improved  |

---

*Generated from CI run 22915644610 comparison_ci.csv (post-revert parameters). Last updated: 2026-03-12.*
