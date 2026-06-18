# Toy experiment: H100 real hardware vs. mgpusim simulated prediction

Goal: show that brute-force reusing an AMD-ISA GPU simulator (mgpusim) to
predict performance on NVIDIA hardware produces numbers far from real
hardware — useful as a motivating example in an introduction section.

All scripts referenced below live in this directory: `scripts/h100_baseline/`.

## Step 1 — Run the kernel on real H100 hardware

Benchmark kernel: `amd/benchmarks/amdappsdk/matrixmultiplication/MatrixMultiplication_Kernels.cl`

On the H100 server:

```bash
sudo apt install ocl-icd-opencl-dev   # OpenCL headers/loader, if missing
./build_and_run.sh 64 64 64 20        # X Y Z iterations
```

Outputs a line like:
```
device=NVIDIA H100 80GB HBM3 X=64 Y=64 Z=64 iterations=20 avg_kernel_seconds=0.0000XXXXX
```
Save this line — it's the real-hardware ground truth for step 3.

To confirm the exact H100 SKU (SXM vs PCIe — changes SM count and clocks):
```bash
nvidia-smi --query-gpu=name,pci.bus_id --format=csv
nvidia-smi -q -d CLOCK
# for exact SM/multiprocessor count, use deviceQuery from the CUDA samples,
# or look up the SM count for the exact product name string printed above
# (e.g. "H100 80GB HBM3" = SXM5, 132 SMs; "H100 PCIe" = 114 SMs)
```

Files: `run_matrixmultiplication.c` (OpenCL host harness), `build_and_run.sh`.

## Step 2 — Run the same problem size through mgpusim

```bash
./run_mgpusim.sh matrixmultiplication mm_64 -x 64 -y 64 -z 64
```
This builds `amd/samples/matrixmultiplication` and runs it in timing mode,
writing `mm_64.sqlite3` (table `mgpusim_metrics`, columns
`Location, What, Value, Unit`). The row `Location='Driver', What='kernel_time'`
is the simulated end-to-end kernel time in seconds.

**Note on problem size:** with the default 64x64x64 matrices, only 1 of 64
CUs is ever active (global size 16x16 / work-group 8x8 = 4 work-groups
total). Use a larger `-x -y -z` if you want the simulated config to actually
be saturated rather than dominated by fixed overhead.

### Where the "SM-like" config is hard-coded (not exposed as a CLI flag)

| Knob | Default | File |
|---|---|---|
| CUs per shader array | 4 | [amd/samples/runner/timingconfig/r9nano/builder.go:80](../../amd/samples/runner/timingconfig/r9nano/builder.go#L80), setter `WithNumCUPerShaderArray` (line 136) |
| Shader arrays per GPU | 16 (64 CUs total) | [r9nano/builder.go:81](../../amd/samples/runner/timingconfig/r9nano/builder.go#L81), setter `WithNumShaderArray` (line 142) |
| CU clock frequency | 1 GHz | [r9nano/builder.go:78](../../amd/samples/runner/timingconfig/r9nano/builder.go#L78), setter `WithFreq` |
| `-gpu mi300a` override | from `mi300a.NumCUPerShaderArray` / `NumShaderArray` | [amd/samples/runner/timingconfig/builder.go:118-122](../../amd/samples/runner/timingconfig/builder.go#L118-L122) |

None of these are wired to a CLI flag today. To drive them from the command
line, add flags in `amd/samples/runner/flag.go` and thread them into the
builder calls in `runner.go` (around the `WithGPUType` call, line 108).

### Important caveat: wavefront/warp width is not a config knob

AMD GCN/CDNA wavefronts are 64 work-items wide; NVIDIA warps are 32 threads
wide. In mgpusim this width literally appears as:

- [amd/kernels/gridbuilder.go:167](../../amd/kernels/gridbuilder.go#L167) — `wavefrontSize := 64`
- [amd/timing/cu/simdunit.go:82-84](../../amd/timing/cu/simdunit.go#L82-L84) — `64 / u.NumSinglePrecisionUnit` cycles to push one wavefront through the SIMD pipeline

These are **not** independent tunables you can set to 32 to "model" an
NVIDIA warp:

- `EXEC`/`VCC` lane masks are `uint64` throughout `amd/emu` because GCN3
  hardware has a 64-bit lane-active mask register; cross-lane instructions
  (`ds_swizzle`, `v_permute`, `dpp`), VGPR/LDS addressing, etc. all assume
  groups of 64 lanes.
- mgpusim's emulator decodes and executes **real GCN3 machine code**
  (HSACO binaries compiled by AMD's compiler for 64-wide wavefronts). An
  OpenCL kernel built for NVIDIA produces PTX/SASS — a different
  instruction set entirely. mgpusim has no NVIDIA front-end/decoder, so
  there is no binary you could feed it that represents what actually runs
  on the H100.

So there is no real "update mgpusim's config to model H100" path — only
matching superficial numbers (CU count ~ SM count). That mismatch is itself
part of the point: even with CU/SM counts matched, simulating real GCN3
machine code with 64-wide wavefronts cannot predict performance of a
different ISA running 32-wide warps. Treat this as a second, structural
reason (alongside any CU/SM scale mismatch) why the prediction is invalid —
worth calling out explicitly in the writeup, not just tuning around.

## Step 3 — Compare

```bash
python3 compare.py \
  --h100-line "device=... avg_kernel_seconds=0.0000XXXXX" \
  --sim-db amd/samples/matrixmultiplication/mm_64.sqlite3
```
Prints H100 time, simulated time, simulated cycle count
(`kernel_time_seconds x freq_hz`, default freq 1e9 — pass `--freq-hz` if you
changed it), and the simulated/real ratio.

## Step 4 — Generalize to other benchmarks

1. Add an entry to `benchmarks.yaml` (sample dir, sim args, H100 harness name/args).
2. Copy `run_matrixmultiplication.c` to a new harness for the new kernel,
   matching its actual argument signature and launch dimensions.
3. Collect H100 results into a text file, one line per benchmark:
   ```
   matrixmultiplication: device=... avg_kernel_seconds=0.0000XXXXX
   ```
4. Run:
   ```bash
   python3 batch_compare.py --h100-results h100_results.txt
   ```
   Prints a summary table (H100 time, simulated time, simulated cycles,
   ratio) for every benchmark with a matching H100 result.

## File index

| File | Purpose |
|---|---|
| `run_matrixmultiplication.c` | OpenCL host harness, runs the real `.cl` kernel on whatever GPU OpenCL finds |
| `build_and_run.sh` | Builds and runs the harness (run on the H100 server) |
| `run_mgpusim.sh` | Builds and runs an `amd/samples/<name>` binary in timing mode |
| `compare.py` | Computes H100-vs-mgpusim gap for one benchmark |
| `benchmarks.yaml` | Registry of benchmarks for the batch pipeline |
| `batch_compare.py` | Runs the mgpusim side and prints a summary table for all registered benchmarks |
| `toy_guidebook.md` | This file |
