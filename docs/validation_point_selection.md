# Validation Point Selection Policy

**Version:** 3.0  
**Status:** Active  
**Supersedes:** Gate benchmark system (accuracy-metrics.md §4)

---

## 1. Motivation

Previous validation used a small number of simulated points per benchmark
(often 5), concentrated at small problem sizes where kernel launch overhead
dominates execution time (the "flat region"). In this regime, absolute
execution times are tiny and nearly constant regardless of problem size,
so low relative errors are misleading — they reflect overhead matching,
not simulation fidelity.

This policy ensures every benchmark is validated with enough points in the
region where compute and memory behaviour actually dominate execution time.

## 2. Definitions

### 2.1 Flat Region (Overhead-Dominated)

A data point belongs to the **flat region** if its hardware execution time
satisfies:

```
hw_time ≤ 2 × min(hw_time)
```

where `min(hw_time)` is the smallest hardware execution time observed for
that kernel across all problem sizes.

**Rationale:** At the smallest problem size, kernel launch overhead
dominates. Points within 2× of this minimum have not yet reached the
regime where compute or memory bandwidth determines execution time.

### 2.2 Non-Flat Region (Compute/Memory-Dominated)

A data point belongs to the **non-flat region** if:

```
hw_time > 2 × min(hw_time)
```

These are the points where the simulator's modeling of compute throughput,
memory bandwidth, cache behaviour, and synchronization actually matters.

### 2.3 Edge Cases

- **All-flat benchmarks:** If every hardware data point satisfies
  `hw_time ≤ 2 × min(hw_time)`, the benchmark is classified as
  "all-flat". This typically means the benchmark never reaches a
  problem size large enough for compute to dominate. The benchmark
  should still be validated, but its accuracy carries less weight.

- **No-flat benchmarks:** If even the smallest point has
  `max(hw_time) / min(hw_time) > 2` and the times are monotonically
  increasing from the start, there is no flat region. All points are
  non-flat.

## 3. Minimum Point Requirements

Point selection rules differ by benchmark tier (see §3.3 for tier
definitions).

### 3.1 Tier 2 — Algorithm Kernels (Flat / Non-Flat Rule)

For Tier 2 benchmarks, the flat/non-flat classification from §2 applies
directly:

| Region | Minimum Points | Condition |
|--------|---------------|-----------|
| **Flat** | ≥ 1 | If flat region exists |
| **Non-flat** | ≥ 5 | If non-flat region exists |

### 3.2 Tier 1 — Microbenchmarks (Auto-Detected Regimes)

Microbenchmarks test specific system parameters and exhibit diverse
response shapes (staircases, saturation curves, cliffs, periodic patterns,
etc.). A single flat/non-flat split is insufficient.

**Point selection process:**

1. **Automated regime detection.** The validation script analyses each
   Tier 1 benchmark's hardware data and identifies regime boundaries
   using a general-purpose change-point / gradient-change algorithm.
   The algorithm does not assume any particular shape — it looks for
   statistically significant changes in the curve's behaviour.

2. **Automated point selection.** The script selects ≥ 2 simulated
   points per detected regime.

3. **Author review.** The script auto-files an issue tagging the
   benchmark author with:
   - The detected regimes (visualised)
   - The selected points
   - A request to approve, adjust, or override

This keeps the process automated for day-to-day CI while ensuring a
human who understands the benchmark validates the regime boundaries.

**Minimum requirements:**

| Condition | Minimum Points |
|-----------|---------------|
| Per detected regime | ≥ 2 |
| Total per benchmark | ≥ 5 |

### 3.3 Tier Definitions

| Tier | What it tests | Litmus test |
|------|--------------|-------------|
| **Tier 1** | Specific system configuration parameters | "If this benchmark is inaccurate, does it tell me **which parameter** to fix?" |
| **Tier 2** | End-to-end algorithm/application timing | Everything else |

See Issue #13 for classification guidance and examples.

### 3.4 Compliance

A benchmark is **compliant** if it meets the minimums for its tier
(where applicable). A benchmark is **non-compliant** if it has simulated
data but does not meet the minimums. Non-compliant benchmarks are still
included in the report but are flagged.

### 3.5 What This Means for CI

For **Tier 2** benchmarks, the CI simulation matrix must include:

1. At least **1 small problem size** (flat region).
2. At least **5 large problem sizes** (non-flat region), where
   `hw_time > 2 × min(hw_time)`.

For **Tier 1** benchmarks, the CI simulation matrix must include:

1. At least **2 points per auto-detected regime**.
2. At least **5 points total**.

If the CI cannot simulate enough sizes (timeout or memory limits), this
is reported as a coverage gap, not a pass.

After automated point selection, the validation script generates a
review issue for the benchmark author. Until the author approves, the
benchmark is marked as **"pending review"** in the report.

## 4. Accuracy Metrics

### 4.1 Per-Benchmark Booster (Tier 2 Only)

The simulator does not model constant overheads such as kernel launch
latency, driver stack, or host-device synchronization. To isolate
simulation fidelity from these systematic offsets, each Tier 2 benchmark
is evaluated with a **per-benchmark booster** — a constant `c` added to
every simulated data point:

```
adjusted_sim(x) = sim(x) + c
```

The booster `c` is the median of `(hw - sim)` across all matched data points
-- the L1-optimal constant that minimises the mean absolute error.
It may be positive or negative.

**Important:** The booster is applied only in the evaluation script, never
in the simulator itself. Its value is reported in the validation report
so reviewers can judge whether the offset is reasonable.

### 4.2 Accuracy Metrics

Two primary metrics are computed **after applying the booster** (for
Tier 2) or directly (for Tier 1):

| Metric | Definition | Target |
|--------|-----------|--------|
| **Average Error** | Mean of per-point `\|adjusted_sim - hw\| / hw` | ≤ 20% (Tier 2), ≤ 10% (Tier 1) |
| **Scaling Factor Error** | See §4.3 | ≤ 20% average across benchmarks |

Additional reported metrics (informational, not gating):

| Metric | Scope | Purpose |
|--------|-------|---------|
| **WMAPE (non-flat)** | Non-flat points only | Weighted accuracy in compute-dominated region |
| **WMAPE (flat)** | Flat points only | Overhead modeling accuracy |
| **Spearman ρ** | All matched points (≥3) | Rank-order accuracy |
| **Booster value** | Per benchmark | Magnitude of systematic offset |

### 4.3 Scaling Factor Error

For academic research, the most important property is that the simulator
correctly predicts **how performance scales** with input size — this is
what papers rely on when reporting speedups and comparing designs.

For each benchmark, a linear regression is performed:

```
time = α × input_size + β
```

on both hardware data and (booster-adjusted) simulation data, using the
matched data points. The **scaling factor** is the slope `α`.

The **scaling factor error** for a benchmark is:

```
scaling_error = |α_sim - α_hw| / |α_hw|
```

The aggregate scaling factor error is the **mean** across all benchmarks
that have ≥ 3 matched data points (minimum for meaningful regression).

**Target:** Average scaling factor error ≤ 20%.

### 4.4 Accuracy Targets

| Tier | Average Error (with booster) | Scaling Factor Error (avg) |
|------|-----------------------------:|---------------------------:|
| **Tier 1** (microbenchmarks) | ≤ 10% | ≤ 20% |
| **Tier 2** (algorithm kernels) | ≤ 20% | ≤ 20% |

**Tier 1 must pass before Tier 2 is evaluated.** If Tier 1
microbenchmarks are inaccurate, Tier 2 errors are likely caused by
incorrect system parameters, not algorithm modeling issues.

### 4.5 Pass / Fail Criteria

A CI run **passes** if:

1. All Tier 1 benchmarks have average error ≤ 10%.
2. All Tier 1 benchmarks are point-compliant (§3.2).
3. The mean Tier 1 scaling factor error ≤ 20%.
4. All Tier 2 benchmarks have average error ≤ 20% (with booster).
5. All Tier 2 benchmarks are point-compliant (§3.1).
6. The mean Tier 2 scaling factor error ≤ 20%.

If Tier 1 fails, Tier 2 is reported but not used for pass/fail.

## 5. Replacing Gate Benchmarks

This policy **replaces** the previous gate benchmark system (atax < 12%,
bicg < 16%, tiled_gemm_16 < 16%). Instead of privileging a few
hand-picked benchmarks:

- **Every benchmark is evaluated equally** using the same framework.
- **No benchmark receives special treatment** in CI pass/fail decisions.
- **Regression detection** uses per-benchmark error and scaling factor
  metrics, not hand-picked thresholds.

## 6. Report Generation

The validation report is auto-generated by:

```bash
python3 scripts/generate_validation_report.py
```

With no arguments, the command uses the default `--scope all` contract and reads
`benchmark-comparison/selected-run-25619929396/comparison_ci.detailed.csv` so the
committed all-scope report can be regenerated from its recorded selected-run
artifact. Use an explicit `--csv benchmark-comparison/comparison_ci.csv` (or
another comparison CSV) only when deliberately targeting current/top-level data;
the generated report, summary, and figures should then preserve that explicit CSV
path as provenance. Curated-scope regeneration (`--scope curated`) keeps the
current top-level `benchmark-comparison/comparison_ci.csv` default and writes
`docs/validation_report_curated.md`.

This script:
1. Resolves the scope-specific default CSV, or the explicit `--csv` override
2. Classifies each point as flat or non-flat
3. Checks compliance with minimum point requirements
4. Fits per-benchmark boosters (Tier 2)
5. Computes average error and scaling factor error
6. Generates per-benchmark data tables and scaling figures
7. Writes the selected report output path

The script is intended to run as part of CI/report regeneration after simulation
or artifact selection completes.
