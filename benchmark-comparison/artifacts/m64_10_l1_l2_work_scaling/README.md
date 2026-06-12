# M64.10 L1/L2 work-scaling simulator evidence

Tracked provenance for issue #2052 on branch `e4-m64-10-produce-value-changing-l1-l2-cache-bandwidth-cur`.

Files:
- `simulator_results.csv`: 6 successful simulator rows, 3 each for `l1_cache_bw` and `l2_cache_bw` with fixed repeat/work-set settings. Each successful run produced 3 `mgpusim_metrics` rows; `sim_ms` is Driver `kernel_time` extracted from `akita_sim_*.sqlite3`.
- `default_repeat_pilot.csv`: earlier default-repeat pilot and timeout/incomplete attempts that motivated the lower fixed repeat count for the representative finishable sweep.
- `commands.sh`: build, simulator run, SQLite extraction, and prepared HIP/MI300A hardware-runner commands.
- `hardware_availability.txt`: local HIP/ROCm availability probe; no hardware rows were collected.

The archived `benchmark-comparison/selected-run-25619929396/` directory was not modified.
