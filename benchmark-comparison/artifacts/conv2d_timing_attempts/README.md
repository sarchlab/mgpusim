# conv2d timing attempts (issue #301)

These artifacts were produced by running all `conv2d` problem sizes (from the old, now-removed `workloads/reference/mi300a.csv` reference) in timing mode on the `mi300a` config.

## Reproduction

From repo root:

```bash
python3 benchmark-comparison/run_conv2d_timing_attempts.py
```

The script:
- builds `amd/samples/conv2d` to `/tmp/conv2d_issue301`
- iterates all 14 `conv2d` sizes from the old `mi300a.csv` (now removed)
- runs each attempt with a fixed timeout of **60 seconds per size**
- writes one raw stdout/stderr artifact per size in this directory
- writes summary CSV: `benchmark-comparison/conv2d_timing_attempts.csv`

## Single-command template

```bash
/tmp/conv2d_issue301 -timing -arch=cdna3 -gpu=mi300a -disable-rtm \
  -N <N> -C <C> -H <H> -W <W> \
  -kernel-height <KH> -kernel-width <KW> -output-channel <OC>
```
