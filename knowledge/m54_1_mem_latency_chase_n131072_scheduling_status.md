# M54.1 / E1 mem_latency_chase N=131072 scheduling status

- Issue / PR: local issue `#884`, local PR `#882`.
- Branch: `e1-m54-1-repair-mem-latency-chase-semantic-equivalence-an`.
- Commit tested: `ebff3762bbe7cdf4d3172197779eb83c7d45de46` (`[Kai] Repair mem latency chase hop semantics`).
- Scope: only the final semantic-equivalent `mem_latency_chase` `N=131072` scheduling status. This does not cover deferred DRAM-region `N>=2097152` behavior or report-cache-hit-rate gating.

## Conclusion

`N=131072` is reclassified as **not WG-scheduling-limited** for the final semantic-equivalent workload.

The current L2-region implementation (`N > 8192`) uses the hardware-default 1024 chase hops and schedules a single 64-thread work-group. Therefore the old many-work-group scheduling concern does not apply to this final workload shape.

## Evidence

Source scheduling at the tested commit:

- `NewBenchmark` defaults `NumChases` to `hardwareDefaultNumHops = 1024`.
- `useRandomChaseMode()` is true for `N=131072` because `131072 > l1ResidentChaseElements (8192)`.
- In random chase mode, `exec()` overrides launch geometry to `blockSize = 64` and `grid = 1`, so `HiddenBlockCountX = 1`.

Command run after `git pull --ff-only origin e1-m54-1-repair-mem-latency-chase-semantic-equivalence-an` reported the branch was already up to date:

```text
.repo_tmp-sam884/bin/mem_latency_chase \
  -timing -arch cdna3 -gpu mi300a -disable-rtm -num_elements 131072
```

Run result:

- Status: completed, not timed out.
- Exit code: `0`.
- Wall time: `18.097 s` under a 300 s Python-enforced timeout.
- SQLite metrics file: `.repo_tmp-sam884/run131072/akita_sim_d7uitp5alt3uq297k31g.sqlite3`.
- `mgpusim_metrics`: `Location=Driver`, `What=kernel_time`, `Value=9.0983e-05 second` (`0.090983 ms` raw kernel time).
- With the current `mem_latency_chase` fixed offset (`0.063 ms`), adjusted time is `0.153983 ms` vs hardware `0.144900 ms` for `N=131072` (`6.268%` relative error). This timing is supporting evidence only; the scheduling conclusion is based on the completed single-WG semantic-equivalent run.
