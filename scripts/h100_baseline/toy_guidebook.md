# Toy experiment: H100 real hardware vs. mgpusim simulated prediction

Goal: show that brute-force reusing an AMD-ISA GPU simulator (mgpusim) to
predict performance on NVIDIA hardware produces numbers far from real
hardware — useful as a motivating example in an introduction section.

All scripts referenced below live in this directory: `scripts/h100_baseline/`.

## Benchmarks covered so far

| Benchmark | `.cl` kernel | mgpusim sample | H100 harness | Args (positional) |
|---|---|---|---|---|
| matrixmultiplication | `amd/benchmarks/amdappsdk/matrixmultiplication/MatrixMultiplication_Kernels.cl` | `amd/samples/matrixmultiplication` | `run_matrixmultiplication.c` / `build_and_run_matrixmultiplication.sh` | `X Y Z [iterations]` — e.g. `64 64 64 20`; mgpusim flags: `-x 64 -y 64 -z 64` |
| matrixtranspose | `amd/benchmarks/amdappsdk/matrixtranspose/native/MatrixTranspose_Kernels.cl` | `amd/samples/matrixtranspose` | `run_matrixtranspose.c` / `build_and_run_matrixtranspose.sh` | `Width [iterations]` — e.g. `256 20`; mgpusim flag: `-width 256` |

To add a new benchmark, append a row here and an entry in `benchmarks.yaml`
(see Step 4).

## Step 1 — Run the kernel on real H100 hardware

On the H100 server, for matrixmultiplication:

```bash
sudo apt install ocl-icd-opencl-dev   # OpenCL headers/loader, if missing
./build_and_run_matrixmultiplication.sh 64 64 64 20   # X Y Z iterations
```

or for matrixtranspose:

```bash
./build_and_run_matrixtranspose.sh 256 20             # Width iterations
```

Each prints a line like:
```
device=NVIDIA H100 80GB HBM3 X=64 Y=64 Z=64 iterations=20 avg_kernel_seconds=0.0000XXXXX
device=NVIDIA H100 80GB HBM3 Width=256 iterations=20 avg_kernel_seconds=0.0000XXXXX
```
Save this line — it's the real-hardware ground truth for step 3.

To confirm the exact H100 SKU and clock (SXM vs PCIe — changes SM count and clocks):
```bash
nvidia-smi --query-gpu=name,pci.bus_id --format=csv
nvidia-smi -q -d CLOCK
# for exact SM/multiprocessor count, use deviceQuery from the CUDA samples,
# or look up the SM count for the exact product name string printed above
# (e.g. "H100 80GB HBM3" = SXM5, 132 SMs; "H100 PCIe" = 114 SMs)
```
The `Graphics`/`SM` clock from `nvidia-smi -q -d CLOCK` (in MHz) is what you
pass to `compare.py --freq_mhz` in step 3.

## Step 2 — Run the same problem size through mgpusim

```bash
./run_mgpusim.sh matrixmultiplication mm_64 -x 64 -y 64 -z 64
./run_mgpusim.sh matrixtranspose mt_256 -width 256
```
This builds the given `amd/samples/<name>` sample and runs it in timing
mode, writing `<out-name>.sqlite3` (table `mgpusim_metrics`, columns
`Location, What, Value, Unit`). The row `Location='Driver', What='kernel_time'`
is the simulated end-to-end kernel time in seconds.

**Note on problem size:** with the default matrixmultiplication 64x64x64
matrices, only 1 of 64 CUs is ever active (global size 16x16 / work-group
8x8 = 4 work-groups total). Use a larger `-x -y -z` (or `-width` for
matrixtranspose) if you want the simulated config to actually be saturated
rather than dominated by fixed overhead.

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
If you do tune these knobs to chase a closer real-vs-simulated ratio,
treat a close match with suspicion rather than as validation — see the
discussion in chat history around the matrixmultiplication 0.8x result for
why multiple simultaneous knob changes can spuriously cancel out.

## Step 3 — Compare

```bash
python3 compare.py \
  --h100-line "device=... avg_kernel_seconds=0.0000XXXXX" \
  --sim-db mm_64.sqlite3 \
  --freq_mhz 1755
```
Prints a 6-row breakdown:
1. GPU frequency (MHz) — the value you passed in.
2. H100 kernel time (real) — straight from the H100 harness's stdout line.
3. H100 cycle count (calculated) — `H100 kernel time x freq_hz`.
4. mgpusim simulated kernel time (real) — straight from the sqlite3 `kernel_time` row.
5. mgpusim simulated cycle count (calculated) — `simulated kernel time x freq_hz`.
6. simulated-time / real-time ratio.

## Step 4 — Generalize to other benchmarks

1. Add a row to the table at the top of this guidebook, and an entry to
   `benchmarks.yaml` (sample dir, sim args, H100 harness name/args).
2. Copy `run_matrixmultiplication.c` or `run_matrixtranspose.c` (whichever
   argument shape is closer) to a new harness for the new kernel, matching
   its actual argument signature and launch dimensions — read the
   benchmark's `.go` file under `amd/benchmarks/amdappsdk/<name>/` to see
   exactly how it computes global/local work sizes and local-memory
   buffer sizes, and mirror that math in the harness.
3. Write a matching `build_and_run_<name>.sh`.
4. Collect H100 results into a text file, one line per benchmark:
   ```
   matrixmultiplication: device=... avg_kernel_seconds=0.0000XXXXX
   matrixtranspose: device=... avg_kernel_seconds=0.0000XXXXX
   ```
5. Run:
   ```bash
   python3 batch_compare.py --h100-results h100_results.txt
   ```
   Prints a summary table (H100 time, simulated time, simulated cycles,
   ratio) for every benchmark with a matching H100 result.

## File index

| File | Purpose |
|---|---|
| `run_matrixmultiplication.c` | OpenCL host harness for matrixmultiplication, runs the real `.cl` kernel on whatever GPU OpenCL finds |
| `build_and_run_matrixmultiplication.sh` | Builds and runs the matrixmultiplication harness (run on the H100 server) |
| `run_matrixtranspose.c` | OpenCL host harness for matrixtranspose |
| `build_and_run_matrixtranspose.sh` | Builds and runs the matrixtranspose harness (run on the H100 server) |
| `run_mgpusim.sh` | Builds and runs an `amd/samples/<name>` binary in timing mode |
| `compare.py` | Computes H100-vs-mgpusim gap for one benchmark |
| `benchmarks.yaml` | Registry of benchmarks for the batch pipeline |
| `batch_compare.py` | Runs the mgpusim side and prints a summary table for all registered benchmarks |
| `toy_guidebook.md` | This file |
