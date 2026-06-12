# MST (Minimum Spanning Tree) — Disabled

This benchmark is disabled from automated runs due to a known race condition
in Borůvka's GPU implementation that causes intermittent failures (exit code 1).

The race occurs during the concurrent edge-contraction phase where multiple
threads may attempt to update the same component representative simultaneously,
leading to non-deterministic incorrect results and assertion failures.

Until the race condition is resolved, this benchmark is skipped in `run_all.sh`.
