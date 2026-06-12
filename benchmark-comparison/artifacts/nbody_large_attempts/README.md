# NBody large-size timing attempts (MI300a size sweep tail)

This directory contains raw timing-attempt artifacts for the four largest
`nbody` sizes from the old hardware reference (now-removed `workloads/reference/mi300a.csv`) that were still missing
attempt evidence after the initial Batch E sweep:

- 49152 particles
- 65536 particles
- 98304 particles
- 131072 particles

## Command template

```bash
amd/samples/nbody/nbody \
  -timing -arch=cdna3 -gpu=mi300a -disable-rtm -particles <SIZE>
```

## Attempt policy

- Simulator timeout per run: **58 seconds**
- Working directories: `/tmp/ares_issue300_nbody/run_<SIZE>`
- Artifacts: `nbody_<SIZE>.log`
- Summary CSV: `benchmark-comparison/nbody_large_attempts_status.csv`

All four attempts timed out at 58s. No additional successful timing rows were
produced for `new_sim_results.csv` / `nbody_large_sim_results.csv`.
