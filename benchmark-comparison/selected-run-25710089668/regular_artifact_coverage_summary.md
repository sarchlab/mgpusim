# MI300A Regular Artifact Coverage Summary

**Evidence label:** `partial_diagnostic_regular_evidence`  
**Complete regular evidence:** `false`

This repo-only summary counts checked-in/materialized regular workflow expectations against observed CSV artifact rows. It does not dispatch workflows, run simulators, collect artifacts, or fabricate outcomes.

## Label interpretation

- `complete_regular_evidence`: every required upstream stage succeeded and simulation, comparison, and regression CSV data-row counts all match the expected regular row count. This is complete regular artifact coverage only, not finishability/provenance completion or proof of simulator accuracy.
- `partial_diagnostic_regular_evidence`: required CSVs are present with recognizable headers but one or more upstream stages are non-success/unknown or a non-empty CSV row count does not match. Treat the artifacts as partial diagnostic regular evidence.
- `insufficient_regular_evidence`: a required CSV is missing, lacks a recognizable header, is empty, or comparison/regression used an empty reference fallback. There is not enough observed artifact evidence to claim regular coverage.

## Expected regular rows

| Tier | Matrix rows | Expected data rows |
|---|---:|---:|
| benchmark-tier1 | 19 | 308 |
| benchmark-tier2 | 63 | 1108 |
| total | 82 | 1416 |

## Upstream stage results

| Stage | Result | Success required for complete evidence |
|---|---|---:|
| validation | `success` | true |
| benchmark-tier1 | `failure` | false |
| tier1-summary | `success` | true |
| benchmark-tier2 | `failure` | false |

## Observed CSV row coverage

| Artifact | CSV path | Data rows | Expected rows | Status |
|---|---|---:|---:|---|
| simulation | `sim_results_ci.csv` | 632 | 1416 | `row_count_mismatch` |
| comparison | `comparison_ci.csv` | 1416 | 1416 | `matches_expected` |
| regression | `regression_ci.csv` | 515 | 1416 | `row_count_mismatch` |

## Evidence reasons

- upstream stage benchmark-tier1 result is failure
- upstream stage benchmark-tier2 result is failure
- simulation CSV sim_results_ci.csv has 632 data rows; expected 1416
- regression CSV regression_ci.csv has 515 data rows; expected 1416
