# Release calibration

The **MI300X Release Calibration** workflow runs on stable MGPUSim releases at
the commit referenced by the release tag. Run it manually for a preview; the
optional benchmark parameter limits a manual run to one benchmark.

The `plan` job opens a run in the hosted calibration report and declares every
selected benchmark. Each fixed-parameter series then runs on one standard Linux
GitHub-hosted runner, with at most 40 series jobs at once. A series processes
increasing scaling values within a shared 5.5-hour simulation budget. When the
budget expires, the active simulation is terminated and the remaining cases are
recorded as unstarted. Each series job writes the points of every case to the
report as soon as the case ends, so results appear while the workflow runs. The
`finalize` job records the final progress and marks the run `completed` when
every series job succeeded, or `failed` otherwise. A configuration counts as
complete only after its points were written. Per-series `results.json` files
and simulator logs remain available as workflow artifacts.

## Live results

Results are at
`https://sarchlab-calibration-report.netlify.app/?run=<run key>`; the `plan`
job links the run from its summary. A release uses the run key
`mgpusim-mi300x-<tag>` and a manual run uses
`mgpusim-mi300x-preview-<workflow run ID>`. A run key must match
`^[A-Za-z0-9_-][A-Za-z0-9._-]{0,199}$`, so a release tag containing other
characters fails the `plan` job.

**Re-run failed jobs** reruns the failed series jobs and `finalize`. A re-run
series job reopens a `failed` run and overwrites the points of its own cases.
A `completed` run cannot be changed. When the workflow is cancelled after
`plan` succeeded, it attempts to run `finalize`, which marks the run `failed`
and keeps the points already written; re-running the cancelled jobs reopens
it. GitHub does not guarantee that `finalize` runs after a cancellation; if it
does not, the run stays `running` and appears stale.

The workflow needs the `CALIBRATION_REPORT_DSN` secret, a PostgreSQL connection
string for the report database's `cr_ingest` role. Every database call uses a
new connection and is retried for about 30 minutes when the connection fails
or the statement times out. In a series job, no write attempt starts more than
345 minutes after the series step starts, which follows checkout, setup and the
build. The cutoff is meant to leave headroom for setup and the artifact uploads
within the 360-minute job timeout, but does not guarantee it. `report_db.py`
contains the writer.

## Reference data

`mi300x-v1.1.json.gz` contains the MI300X ROCm 10.2 remeasurement completed on
November 23, 2026 EST, using GPU Benchmarks revision
`a8ca84f8ce4373b3c091d4dc97a8f067607847a9`. It contains 2,861 configurations
across 164 series and 455,291 median dispatch timings. Configurations are
limited to the existing MGPUSim replay plan with successful hardware kernel
traces. Eleven otherwise supported configurations lacked kernel timings.
Transfer-only and unsupported cases are outside this comparison.

Each reference stores exact simulator sample arguments and the median time of
each matched hardware launch across successful repetitions. Regenerate the v1.1
reference from the archived simulator replay plan and the verified MI300X
collection archives with Python and zstd.

```sh
python3 gpu_perf_scripts/calibration/release/build_reference.py \
  /path/to/plan-full.json /path/to/verified-archives \
  gpu_perf_scripts/calibration/release/mi300x-v1.1.json.gz
```

The workflow reads every checked-in reference and concatenates their series in
the order of `REFERENCES` in `workflow.py`, v1.1 first. A series is identified
by its position, so a new reference only appends series. Pass `--reference`
once per file to use other files.

### Supplementary `code_fetch_latency` collection

`mi300x-code-fetch-latency.json.gz` adds 52 configurations in four series: 13
loop-body sizes for each of 1, 4, 8 and 16 loops. `code_fetch_latency` was
added to GPU Benchmarks after v1.1, so these timings are not part of the v1.1
collection. They were collected on November 26, 2026 UTC from GPU Benchmarks
revision `9678825670a00099b575560b3695aec029852c58`, built with ROCm 10.2.0 on
an AMD Developer Cloud instance that reports an "AMD Instinct MI300X VF". That
machine is not the one of the v1.1 collection.

Each configuration ran in 20 fresh processes. A process queries the kernel's
attributes, runs the suite's vector-add warmer and launches the kernel once.
The reference stores the median dispatch duration that `rocprofv3
--kernel-trace` records for that launch. The report names the hardware value of
this benchmark "MI300X VF kernel time (median, supplementary collection)". The
file records the benchmark revision, machine, toolchain, driver and profiler
versions, the timing source and the SHA-256 of its raw input next to the
series.

Build it from a CSV with one row per repetition, kept outside the repository:

```sh
python3 gpu_perf_scripts/calibration/release/build_code_fetch_latency_reference.py \
  /path/to/reference_raw.csv \
  gpu_perf_scripts/calibration/release/mi300x-code-fetch-latency.json.gz \
  --benchmark-revision 9678825670a00099b575560b3695aec029852c58 \
  --machine "AMD Instinct MI300X VF" --provider "AMD Developer Cloud" \
  --rocm-version 10.2.0 --collected-at 2026-11-26T17:55:51+00:00 \
  --field driver_version=7.1.3.31500000 --field "profiler=rocprofv3 1.4.2"
```

Repeat `--field NAME=VALUE` for further provenance. The CSV needs `body_ops`,
`loops` and `rep`, and the duration either as the profiler's `Start_Timestamp`
and `End_Timestamp`, as `Duration_ns` or as `dispatch_ns`. With the profiler's
`Kernel_Name`, `Grid_Size_*` and `Workgroup_Size_*` columns the launch is taken
from the trace and checked; without them the benchmark's default launch of one
64-thread block is assumed. A row is dropped unless each of these columns, when
present, has the value shown: `status=ok`, `rc=0`, `n_dispatch=1`,
`n_fetch_dispatches=1`, `op=fma`.

## Comparison

The simulator records individual dispatch intervals when
`-report-kernel-dispatches` is enabled. A comparison requires matching kernel
names, launch order and grid and workgroup dimensions. The report shows
verification failures with a warning and includes their timing when it is
available and matches the hardware launch sequence. For `cache_latency`,
pairing omits the simulated cache primer from the comparison when both simulated
pointer-chase launches match the single measured hardware launch's name and
geometry.

Some benchmarks label repeated dispatches with a `launch_group` non-scaling
parameter. The labels depend only on the hardware reference, so they are
declared before any simulation runs. A benchmark is labelled only when every
reference series of it passes the checks; otherwise all its series
keep individual launch labels. The original `launch` labels, per-dispatch
values, and statuses are preserved. A simulated sequence that does not match
marks the affected points `unsupported` without changing their labels.

ALTIS CFD is labelled when each configuration has the expected 80-dispatch
sequence of ten timesteps with common launch geometry. Pairing
recognizes the simulator's three unqualified CFD C symbols as aliases for the
corresponding measured `cfd_device` names. Copy, step factor, and stage-0 flux
separate the first timestep from later timesteps. Flux and update each group
stages 1 and 2 together.

Rodinia Pathfinder, Parboil LBM, PolyBench FDTD2D, and PolyBench Jacobi2D are
labelled when each configuration has the launch count, phase order, and grid
and workgroup dimensions implied by its parameters. Their labels separate each
phase's first launch from later launches and mark a shorter final Pathfinder
launch separately.

## Tests

Run `python -m unittest` in this directory with Python 3.12, `psycopg` 3 and
`c++filt`. `test_report_db_integration.py` runs only when
`CALIBRATION_REPORT_TEST_DSN` (the `cr_ingest` login) and
`CALIBRATION_REPORT_TEST_ADMIN_DSN` (a role that can read schema `cr`) point at
a database with the calibration-report migrations applied.
