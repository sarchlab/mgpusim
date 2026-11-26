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

## Collecting a reference

`collect_hardware.py` produces the archives that `build_reference.py` reads. It
needs Python 3.9 or later, `zstd`, and a built GPU Benchmarks checkout; it does
not build the benchmarks.

```sh
# 1. Cases, parameters and expected launches, taken from existing references.
python3 collect_hardware.py plan mi300x-v1.1.json.gz -o plan.json

# 2. On the GPU machine: one fresh process per repetition.
python3 collect_hardware.py run plan.json --benchmarks /path/to/gpu_benchmarks \
  --output collection --reps 7 --sclk-max 1680

# 3. Archives for build_reference.py.
python3 collect_hardware.py pack collection verified-archives
```

`run` can be interrupted and started again; repetitions that already succeeded
are kept. A repetition that exceeds `--timeout` is recorded as `timeout`, its
whole process group is killed, and the larger cases of the same series are
recorded as `skipped_after_timeout`. `--benchmark NAME` limits a run to one
benchmark, and `--set NAME=VALUE` adds a benchmark parameter to every case, for
example `--set vector_add_warmup=false`.

On ROCm the timer is `rocprofv3 --kernel-trace`: every kernel dispatch of the
run is kept in `all-dispatches.csv`, and the measured ones go to
`measured-dispatches.csv`. A case taken from an earlier reference must show the
launches that reference recorded, by kernel name and launch shape, in the same
order; otherwise the repetition is recorded as `launch_mismatch`. A case
without expected launches keeps every dispatch except the suite's clock warmer
and the runtime's buffer fill, or the dispatches matching its `kernel_pattern`.
`--timer reported` uses the GPU event times the benchmark prints instead. It is
the default on CUDA, records no launch shape, and sums repeated launches of a
kernel, so it is suited to checking a collection setup, not to a reference.

`--sclk-max` caps the AMD GPU core clock with `amd-smi` for the whole run and
restores the previous limit and performance level afterwards. The run fails to
start if the cap does not take effect. The core clock and power are sampled
throughout into `clock-monitor.csv` (sysfs on ROCm, `nvidia-smi` on CUDA), and
each log row carries the highest clock sampled during that repetition; a row is
marked `core_clock_above_limit` when it exceeds the cap by more than 2%.
`environment.jsonl` records the host, tool versions, GPU Benchmarks revision,
timer, clock cap and extra parameters of every `run` invocation.

The MI300X v1.1 reference was collected with the core clock at about 1680 MHz:
capping an "AMD Instinct MI300X VF" instance (default 2100 MHz) at 1680 MHz
reproduces its `fp32_throughput` and `empty_kernel` times to within 0.3%.

`build_reference.py` still writes the GPU Benchmarks revision and completion
time of the v1.1 collection into its output; change them when building a
reference from a new collection.

## Tests

Run `python -m unittest` in this directory with Python 3.12, `psycopg` 3 and
`c++filt`. `test_report_db_integration.py` runs only when
`CALIBRATION_REPORT_TEST_DSN` (the `cr_ingest` login) and
`CALIBRATION_REPORT_TEST_ADMIN_DSN` (a role that can read schema `cr`) point at
a database with the calibration-report migrations applied.
