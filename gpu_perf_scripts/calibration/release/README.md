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

Every MI300X timing sample and release calibration uses the same 1680 MHz
GPU preset, the approximate graphics clock of the v1.1 hardware measurements.
The preset drives the CUs, caches and command dispatcher. DRAM has its own
1 GHz clock. The existing cycle counts for caches and dispatch now translate
to longer times than under the previous 2100 MHz preset.

### LDS slope regression

The gfx942 CU models 32 four-byte LDS banks, with broadcasts and conflicts
computed separately inside each instruction phase. The CU reserves bank
service and SIMD-pair transport separately, so other waves can overlap
the outstanding result latency. Generic FLAT through the shared aperture
uses the DS phase schedule and transports a 64-bit lane address.
The FLAT transport and the fixed 44-cycle `cu.Spec.LDSReturnCycles` are
effective estimates from the existing dependent loop. The widest reads
and writes use different lane groups.

To check the eight single-workgroup series, build the sample and use the
release workflow's Python dependencies (no report database is needed).

```sh
go build -o /tmp/shared_mem_bandwidth ./amd/samples/shared_mem_bandwidth
python3 gpu_perf_scripts/calibration/release/check_lds_slopes.py /tmp/shared_mem_bandwidth
```

The check verifies each output, matches the recorded kernel and geometry,
and fits duration against 64, 128 and 256 inner iterations with a free
intercept on both sides. Each simulated slope must be within 5% of the
hardware slope. The table reports microseconds per iteration.

### Regenerating the reference

The release reference stores exact simulator sample arguments and the median
time of each matched hardware launch across successful repetitions. Regenerate
it from the archived simulator replay plan and the verified MI300X collection
archives with Python and zstd.

```sh
python3 gpu_perf_scripts/calibration/release/build_reference.py \
  /path/to/plan-full.json /path/to/verified-archives \
  gpu_perf_scripts/calibration/release/mi300x-v1.1.json.gz
```

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
