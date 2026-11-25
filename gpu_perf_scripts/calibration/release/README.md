# Release calibration

The **MI300X Release Calibration** workflow runs on stable MGPUSim releases at
the commit referenced by the release tag. It assigns each fixed-parameter series
to one standard Linux GitHub-hosted runner and runs at most 40 series jobs at
once. Each series processes increasing scaling values within a shared
5.5-hour simulation budget. When the budget expires, the active simulation is
terminated and the remaining cases are recorded as unstarted. Independent
series and completed results are preserved.

Run the workflow manually to get a report artifact without publishing. The
optional benchmark parameter limits a manual run to one benchmark. The report
job exports a single `data.json` and packages it with the published npm viewer
using `npx --yes calibration-report@0.3.0 --data data.json --out site`.

## Preview an active run

Run the collector on a machine with authenticated GitHub CLI access to the
repository. It reads only the plan and result artifacts. Each completed series
appears at the next poll. An in-flight series is pending until its result
artifact is uploaded. The standard report refreshes its JSON every ten seconds.

```sh
gh run list --workflow mi300x_release_calibration.yml
template=$(mktemp -d)
npm install --prefix "$template" --no-save --ignore-scripts calibration-report@0.3.0
python3 gpu_perf_scripts/calibration/release/preview.py RUN_ID \
  "$template/node_modules/calibration-report/dist" /path/to/preview/RUN_ID \
  --publish marin-5:.cache/mgpusim/calibration-previews/RUN_ID/
```

The collector polls GitHub every 60 seconds by default and exits after its
final update. The optional `--publish` destination must already be served over
the Tailnet and is updated with rsync over SSH. The example destination is available at
`http://marin-5.tail13b378.ts.net:18840/RUN_ID/`. Omit it to serve the output
locally. Use a separate output directory for each run.

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
available and matches the hardware launch sequence. The standard report
excludes those points from accuracy statistics. For `cache_latency`, the
report omits the simulated cache primer from the comparison when both simulated
pointer-chase launches match the single measured hardware launch's name and
geometry. The release and live preview each write one `data.json` with all
available dispatches and use the published Calibration Report viewer. The viewer
controls summaries, chart rendering, and pagination.
When switching an older preview to this layout, use a fresh output directory.

ALTIS CFD includes an optional `launch_group` non-scaling parameter when its
80-dispatch sequence matches the expected ten timesteps with common launch
geometry. Pairing recognizes the simulator's three unqualified CFD C symbols
as aliases for the corresponding measured `cfd_device` names. Copy, step
factor, and stage-0 flux separate the first timestep from
later timesteps. Flux and update each group stages 1 and 2 together. The
original `launch` labels, per-dispatch values, and statuses are preserved.
A sequence mismatch leaves all records in that series without group annotations.

Rodinia Pathfinder, Parboil LBM, PolyBench FDTD2D, and PolyBench Jacobi2D also
label repeated launches. The exporter checks the reference launch count, phase
order, and grid and workgroup dimensions for each configuration before adding
the labels. It separates each phase's first launch from later launches and
labels a shorter final Pathfinder launch separately. If a reference series
fails those checks, the benchmark falls back to individual launch labels. The
labels stay stable as simulator artifacts arrive. All per-dispatch values and
existing timing and status checks are preserved.

## GitHub Pages setup

Before the first release that uses this workflow, set **Settings → Pages →
Build and deployment → Source** to **GitHub Actions**. Allow release tags to
deploy through the `github-pages` environment if it has branch or tag
restrictions. The workflow uses the built-in GitHub token.

The publish job adds `calibration/releases/<tag>/` to the existing `gh-pages`
branch and stores each new dataset there as `data.json.gz` to stay within
GitHub's per-file size limit. Before uploading the Pages artifact, the job
copies the full committed site to a temporary directory and restores each
compressed release dataset to `data.json`. Earlier reports with plain JSON
remain available, and every report continues to serve `data.json` at its
versioned URL. A branch push publishes only when a Pages deployment workflow
runs.
