# Release calibration

The **MI300X Release Calibration** workflow runs on stable MGPUSim releases at
the commit referenced by the release tag. It assigns each fixed-parameter series
to one standard Linux GitHub-hosted runner and runs at most 40 series jobs at
once. Each series processes increasing scaling values within a shared
5.5-hour simulation budget. When the budget expires, the active simulation is
terminated and the remaining cases are recorded as unstarted. Independent
series and completed results are preserved.

Run the workflow manually to get a report artifact without publishing. The
optional benchmark parameter limits a manual run to one benchmark.

## Preview an active run

Run the collector on a machine with authenticated GitHub CLI access to the
repository. It reads only the plan and result artifacts. Each completed series
appears at the next poll. An in-flight series is pending until its result
artifact is uploaded. The standard report refreshes its JSON every ten seconds.

```sh
gh run list --workflow mi300x_release_calibration.yml
python3 gpu_perf_scripts/calibration/release/preview.py RUN_ID \
  /path/to/calibration-report/site /path/to/preview/RUN_ID \
  --publish marin-5:.cache/mgpusim/calibration-previews/RUN_ID/
```

Use the template revision pinned by the release workflow. The collector polls
GitHub every 60 seconds by default and exits after its final update. The
optional `--publish` destination must already be served over the Tailnet and is
updated with rsync over SSH. The example destination is available at
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
geometry. Small runs use a single
unmodified Calibration Report template. For larger runs, a separate directory
links to an overview with all results and to each benchmark report. Long launch
sequences are split into numbered pages with explicit launch ranges. Each page
uses the same unmodified template.

## GitHub Pages setup

Before the first release that uses this workflow, set **Settings → Pages →
Build and deployment → Source** to **GitHub Actions**. Allow release tags to
deploy through the `github-pages` environment if it has branch or tag
restrictions. The workflow uses the built-in GitHub token.

The publish job adds `calibration/releases/<tag>/` to the existing `gh-pages`
branch and deploys the full branch contents with GitHub Pages. This retains
earlier release reports and existing site content. After the source switch, a
branch push publishes only when a Pages deployment workflow runs.
