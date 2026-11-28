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

`mi300x-v1.2.json.gz` is the main reference. It holds the GPU Benchmarks v1.2
tier 1/2 collection, taken from GPU Benchmarks revision
`7f2c5552a43b16ad95b90fed88e61f0c80c5236b` with ROCm 10.2.0 (amdgpu 7.1.3) on
an AMD Developer Cloud instance that reports an "AMD Instinct MI300X VF". The
repetitions ran from November 26, 2026 22:31 UTC to November 28, 2026 01:21
UTC. The GPU core clock was capped at 1680 MHz with
`amd-smi set -L sclk max 1680`; the highest core clock sampled during any
repetition was 1691 MHz. Each configuration ran in 7 fresh processes with the
suite's vector-add warmer on, and the times are the dispatch durations that
`rocprofv3 --kernel-trace` records.

The reference keeps the 2,861 configurations of the earlier v1.1 reference,
with the same case numbers, simulator arguments and series order: 164 series
and 455,291 median dispatch timings. Every configuration had 7 successful
repetitions with one launch sequence, so none was dropped. The other v1.2
configurations have no simulator arguments yet and are not part of it.
Transfer-only and unsupported cases are outside this comparison. The file
records the benchmark revision, collection start and end, clock cap, timer and
plan hash next to the series, and lists dropped cases (none).

Short kernels, under about 10 µs, are about 2–3 µs slower in v1.2 than in the
v1.1 collection (GPU Benchmarks `a8ca84f8`, November 2026, on another MI300X);
the cause is unknown, and v1.1 was inconsistent at those sizes. The difference
is clearest for the single-launch throughput microbenchmarks
(`fp16_throughput`, `fp32_throughput`, `fp64_throughput`, `int32_throughput`:
median +1.9 to +2.7 µs); short launches inside long launch sequences, such as
Pathfinder, Gaussian and LUD, are mostly unchanged. The device code of these
benchmarks is the same in both revisions.

Every MI300X timing sample and release calibration uses the same 1680 MHz
GPU preset, the core clock cap of the hardware collection.
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

Each reference stores exact simulator sample arguments and the median time of
each matched hardware launch across successful repetitions. Regenerate the
main reference from the collection plan and the collection directory with
Python 3.12. The collection holds `collection.json`, `collection-current.log`
and, for each successful repetition,
`dispatch/<benchmark>/<case>/<rep>/trace/measured-dispatches.csv`.

```sh
python3 gpu_perf_scripts/calibration/release/build_reference.py \
  /path/to/plan_v1.2_tier12.json /path/to/collection_v1.2 \
  gpu_perf_scripts/calibration/release/mi300x-v1.2.json.gz
```

Only plan cases with simulator `args` are used. The builder checks that the
collection ran this plan (its SHA-256) and that each case measured the
launches the plan expects. A case is dropped when it has no successful
repetition or its repetitions disagree on the launch sequence.

The workflow reads every checked-in reference and concatenates their series in
the order of `REFERENCES` in `workflow.py`: the main reference first, then
`code_fetch_latency`, then the loop kernels. A series is identified by its
position, so a new reference only appends series. Pass `--reference` once per
file to use other files. The release workflow runs one job per series and
GitHub allows at most 256, so the references together must stay within 256
series; they have 228.

### Supplementary `instruction_cost` and `loop_wait` collection

`mi300x-loop-kernels.json.gz` adds 1,564 configurations in 60 series for the
`instruction_cost` and `loop_wait` samples, each a single measured `ic_*` or
`lw_*` kernel. They are 782 kernel and launch combinations, each at 4,096 and
16,384 loop iterations:

- `instruction_cost`, 988 configurations in 52 series: 320 v1.2 cases of the
  original bodies at 64 and 256 threads (from cases 100189–100956), and 668
  cases of a second collection with the new pair, scalar and branch bodies and
  512 and 1024 threads.
- `loop_wait`, 576 configurations in 8 series, all from the second collection.

Both collections also measured 65,536 and 262,144 iterations. The builder
leaves those out (`MAX_ITERATIONS`, option `--max-iterations`, recorded in
the file as `max_iterations`): two points per combination give the cycles per
iteration, and the longer points would take about three quarters of the
simulation time, which the cap reduces from about 29 to about 1.7 laptop
hours. The full measurements stay in the collections.

The second collection used the same instance, collector, clock cap and
protocol from November 28, 2026 01:26 to 02:44 UTC, with GPU Benchmarks at
`cd5f922`, whose kernel sources are those merged in GPU Benchmarks `8355c041`
(#204). Each of its cases selected the measured kernel by name; in the v1.2
cases the builder drops the `__amd_rocclr_copyBuffer` launch that follows it.
`instruction_cost` with chained dependency, 64 threads and 2 to 16 copies of
the eight single VALU instructions is in both collections: 128 configurations
over the four iteration counts, which agree within 0.5% (median 0.02%), 64 of
them within the cap. The reference uses the second collection's measurement,
taken in the same session as `loop_wait`, and lists the replaced v1.2 cases.

A series is one benchmark, instruction and launch shape. The dependency or
layout and the copies per iteration are parameters of each case, which the
report shows like series parameters; grouping them keeps the workflow within
256 jobs. Each series runs its cases in order of increasing iterations. The
report names the hardware value of these benchmarks "MI300X VF kernel time
(median, supplementary collection)". The file records the provenance of both
collections.

```sh
python3 gpu_perf_scripts/calibration/release/build_loop_kernel_reference.py \
  /path/to/plan_v1.2_tier12.json:/path/to/collection_v1.2 \
  /path/to/plan_i203.json:/path/to/collection_i203 \
  gpu_perf_scripts/calibration/release/mi300x-loop-kernels.json.gz
```

A configuration measured by a later collection replaces an earlier one. The
builder checks that each measured kernel and launch shape is the one the
sample launches for the recorded arguments.

### Supplementary `code_fetch_latency` collection

`mi300x-code-fetch-latency.json.gz` adds 52 configurations in four series: 13
loop-body sizes for each of 1, 4, 8 and 16 loops. They are the
`code_fetch_latency` cases (100137–100188) of the same GPU Benchmarks v1.2
collection as the main reference, taken on November 27, 2026 UTC. The file
was built before the main reference, from a per-repetition CSV of these
cases.

Each configuration ran in 7 fresh processes. A process queries the kernel's
attributes, runs the suite's vector-add warmer and launches the kernel once.
The reference stores the median dispatch duration that `rocprofv3
--kernel-trace` records for that launch. The report names the hardware value of
this benchmark "MI300X VF kernel time (median, supplementary collection)". The
file records the benchmark revision, machine, clock cap, toolchain, driver and
profiler versions, the timing source and the SHA-256 of its raw input next to
the series.

Build it from a CSV with one row per repetition, kept outside the repository:

```sh
python3 gpu_perf_scripts/calibration/release/build_code_fetch_latency_reference.py \
  /path/to/reference_raw_1680mhz.csv \
  gpu_perf_scripts/calibration/release/mi300x-code-fetch-latency.json.gz \
  --benchmark-revision 7f2c5552a43b16ad95b90fed88e61f0c80c5236b \
  --machine "AMD Instinct MI300X VF" --provider "AMD Developer Cloud" \
  --rocm-version 10.2.0 --collected-at 2026-11-27T02:32:41+00:00 --repetitions 7 \
  --field collection_started_at=2026-11-27T02:31:01+00:00 \
  --field core_clock_limit_mhz=1680 \
  --field hip_version=7.17.26434 \
  --field "compiler=AMD clang 24.0.0git bdc35841c97ff51400273b91d7bc05c96e19c2d9" \
  --field driver_version=7.1.3.31500000 \
  --field "profiler=rocprofv3 1.4.2" \
  --field "protocol=Each repetition is a fresh process with one cold launch: kernel attribute query, the suite's vector-add warmer, then the timed launch. The GPU core clock was capped at 1680 MHz with amd-smi set -L sclk max 1680." \
  --field "collection=GPU Benchmarks v1.2 tier 1/2 collection, plan sha256 637cf81b36e748e4949ee144489b059f1d4aaa269a70930aa9c6465aa794d945, cases 100137-100188"
```

This is the command that produced the committed file, with Python 3.12; the
fields are recorded in the order given. The CSV needs `body_ops`, `loops` and
`rep`, and the duration either as the profiler's `Start_Timestamp` and
`End_Timestamp`, as `Duration_ns` or as `dispatch_ns`. With the profiler's
`Kernel_Name`, `Grid_Size_*` and `Workgroup_Size_*` columns the launch is taken
from the trace and checked, and a blank one is an error; without those columns
the benchmark's default launch of one 64-thread block is assumed. A row is
dropped unless each of these columns, when present, has the value shown:
`status=ok`, `rc=0`, `n_dispatch=1`, `n_fetch_dispatches=1`, `op=fma`.

The builder writes nothing unless the collection is complete: all 13 body
sizes for all four loop counts, each with `--repetitions` kept rows (20 by
default), and every row within the limits of the sample.

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
