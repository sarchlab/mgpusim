# MI300X simulation sweeps

Use the **MI300X Calibration** workflow in GitHub Actions to run one job per
enabled benchmark. Small and medium sweeps use GitHub-hosted runners. The large
and x-large settings use self-hosted runners. Each job uploads a verified timing
CSV and a JSON snapshot containing all points and their run statuses.

Run the same sweep locally or on a simulation host from the desired MGPUSim
checkout. Python 3 and the Go toolchain declared in go.mod are required.

```sh
PER_RUN_TIMEOUT=3600 python3 gpu_perf_scripts/calibration/run_sweep.py \
  empty_kernel empty_kernel_sim_results.csv --ref ground_truth.csv \
  --snapshot empty_kernel_sweep.json
python3 gpu_perf_scripts/calibration/export_live_report.py empty_kernel_sweep.json report
```

Use the `benchmark,scaling_param_name,scaling_param_value,non_scaling_json,kernel_ms_mean`
columns for a new reference CSV. The checked-in CSV is an earlier reference
collection. To replay exact arguments from a newer collection, pass
`--plan plan.json --reference reference.json` in place of `--ref`. The plan is
an array of cases; the reference is an object keyed by case ID. For example:

```json
[{"case":156,"benchmark":"empty_kernel","sample":"empty_kernel","axis":"num_blocks","x":1,"series":"block_size=32","args":["-num-blocks=1","-block-size=32"]}]
```

```json
{"156":{"kind":"kernel","hardware_ms":0.001}}
```

Each fixed-parameter series runs in increasing scaling-parameter order. A
timeout skips larger points in that series. Other series keep running.
Set `SWEEP_STOP_ON_TIMEOUT=0` to continue after timeouts. `SWEEP_TIMEOUT`
limits when a sweep can start a new point.

The CSV supplies verified timings to the existing dashboard. The JSON snapshot
also preserves a kernel time for verification failures when the simulator wrote
a valid Driver metric. The shared Calibration Report displays the timing with
a warning and omits it from accuracy summaries. Use `--add-snapshot file.json`
on the exporter to combine another benchmark snapshot, and `--watch 10` to
republish as files change.
