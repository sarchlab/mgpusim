# MGPUSim NVIDIA

A trace-driven simulator for NVIDIA GPUs (H100, A100) built on Akita. It
replays the SASS instruction trace of a real CUDA program, collected with the
[Accel-Sim](https://github.com/accel-sim/accel-sim-framework) NVBit tracer,
through a model of the GPU: the thread block dispatcher, SMs, SM sub-partitions
(warp schedulers, per-opcode pipelines, scoreboards), per-SMSP L1 caches, L2
cache banks, and DRAM. It reports the simulated execution time.

> [!WARNING]
> The NVIDIA simulator is experimental and not reliable yet. Its simulated
> times are not validated against hardware and can be wrong, even as relative
> numbers. Do not draw performance conclusions from it. We will remove this
> warning once we consider the NVIDIA simulator ready.

## Layout

| Path | What it is |
| --- | --- |
| `nvidia.go` | The simulator command (`go run ./nvidia`). |
| `tracecollector/` | Runs a CUDA program under the NVBit tracer (trace) and under Nsight Compute (profile of the real GPU). |
| `benchmarks/` | CUDA host programs for benchmarks whose kernels live in `amd/benchmarks`. |
| `trace/` | Reader for `kernelslist.g` and `.traceg` files. |
| `platform/` | H100 and A100 configurations. |
| `driver/`, `gpu/`, `sm/`, `smsp/`, `message/` | The simulated components. |
| `runner/`, `benchmark/` | Load a trace, build the platform, run the simulation. |

### Shared benchmark kernels

The CUDA benchmarks do not carry their own kernels. Each host program in
`benchmarks/` `#include`s the HIP kernel source from `amd/benchmarks/.../native/`,
and `benchmarks/include/hip/hip_runtime.h` maps the few HIP names those kernels
use (`hipThreadIdx_x`, `hipBlockIdx_x`, ...) to CUDA. The AMD and NVIDIA
simulations of a benchmark therefore run the same kernel code.

| Suite | Benchmarks |
| --- | --- |
| PolyBench/GPU (13) | [`atax`](benchmarks/polybench/atax/SOURCE.md), [`bicg`](benchmarks/polybench/bicg/SOURCE.md), [`conv2d`](benchmarks/polybench/conv2d/SOURCE.md), [`conv3d`](benchmarks/polybench/conv3d/SOURCE.md), [`correlation`](benchmarks/polybench/correlation/SOURCE.md), [`fdtd2d`](benchmarks/polybench/fdtd2d/SOURCE.md), [`gemm`](benchmarks/polybench/gemm/SOURCE.md), [`gramschmidt`](benchmarks/polybench/gramschmidt/SOURCE.md), [`jacobi2d`](benchmarks/polybench/jacobi2d/SOURCE.md), [`mvt`](benchmarks/polybench/mvt/SOURCE.md), [`syr2k`](benchmarks/polybench/syr2k/SOURCE.md), [`threemm`](benchmarks/polybench/threemm/SOURCE.md), [`twomm`](benchmarks/polybench/twomm/SOURCE.md) |
| Rodinia (9) | [`backprop`](benchmarks/rodinia/backprop/SOURCE.md), [`gaussian`](benchmarks/rodinia/gaussian/SOURCE.md), [`hotspot`](benchmarks/rodinia/hotspot/SOURCE.md), [`hotspot3d`](benchmarks/rodinia/hotspot3d/SOURCE.md), [`lavamd`](benchmarks/rodinia/lavamd/SOURCE.md), [`lud`](benchmarks/rodinia/lud/SOURCE.md), [`nw`](benchmarks/rodinia/nw/SOURCE.md), [`pathfinder`](benchmarks/rodinia/pathfinder/SOURCE.md), [`srad`](benchmarks/rodinia/srad/SOURCE.md) |

That is every benchmark in `amd/benchmarks/polybench` and `amd/benchmarks/rodinia`.
Each benchmark directory has a `SOURCE.md` with the kernel source path, the
commit and date of the kernel and of the Go host code it follows, the port date,
the usage line with the default sizes (the same flags and defaults as the
matching `amd/samples` program unless noted), and any difference from the AMD
host code.

To add a benchmark, create `benchmarks/<suite>/<name>/<name>.cu` that includes
the kernel file from `amd/benchmarks/<suite>/<name>/native/`; the Makefile picks
it up automatically. Extend the shim header if the kernel uses other HIP names.
If the shared kernels move (for example to a top-level `benchmarks/`),
build with `make KERNEL_ROOT=<new path>`.

## Workflow

Steps 1 to 4 run on a Linux server with an NVIDIA GPU. Step 5 can run
anywhere Go runs, including the same server.

You need:

- Go (the version in the repository's `go.mod`; `go` downloads it on first use)
- On the GPU server: the CUDA toolkit (`nvcc`, and `ncu` for profiling), `make`, `git`, `xz`

All commands below run from the root of the mgpusim repository.

### 1. Build the simulator

```bash
go build -o nvidia/nvidia ./nvidia
./nvidia/nvidia -h
```

### 2. Build the CUDA benchmarks

```bash
make -C nvidia/benchmarks            # H100 (sm_90)
# make -C nvidia/benchmarks ARCH=sm_80   # A100
```

This creates one binary per benchmark, named `<suite>-<benchmark>`, in
`nvidia/benchmarks/bin/` (for example `bin/polybench-atax` and
`bin/rodinia-pathfinder`). `make -C nvidia/benchmarks list` prints the names,
and `make -C nvidia/benchmarks bin/polybench-gemm` builds just one. Each program checks its
own result. Run one natively to check the GPU and the build; it should print
`Passed!`:

```bash
./nvidia/benchmarks/bin/polybench-atax -x 256 -y 256
```

### 3. Get the Accel-Sim NVBit tracer (once per server)

You need two tools from Accel-Sim: the tracer `tracer_tool.so` and the
post-processor `post-traces-processing`. Either of these works:

- **Tools you already have.** If the server has the pair that mnt-collector used
  (its `lib/tracer_tool.so` and `lib/post-traces-processing`), use them.
- **Build them.** Clone Accel-Sim outside the mgpusim repository and build its
  tracer (see Accel-Sim's `util/tracer_nvbit/README.md` if the build fails):

  ```bash
  git clone -b dev https://github.com/accel-sim/accel-sim-framework.git ~/accel-sim-framework
  cd ~/accel-sim-framework/util/tracer_nvbit
  export CUDA_INSTALL_PATH=/usr/local/cuda   # adjust to your CUDA install
  ./install_nvbit.sh
  make
  cd -
  ```

Point the trace collector at the two tools, for example:

```bash
export TRACER_TOOL=~/accel-sim-framework/util/tracer_nvbit/tracer_tool/tracer_tool.so
export TRACE_PROCESSOR=~/accel-sim-framework/util/tracer_nvbit/tracer_tool/traces-processing/post-traces-processing
```

Older and newer tracer builds differ in file names and formats. The trace
collector handles both:

| | Older builds (e.g., mnt-collector's) | Newer builds (`dev` branch) |
| --- | --- | --- |
| Raw kernel list | `traces/kernelslist` | `traces/kernelslist_ctx_<ctx>` |
| Trace version | 5 | 6 (adds register values; the collector sets `ALLOW_REG_VAL_TRACING=0` to get 5) |
| Post-processor output | `.traceg` (text) | `.tracez` unless given `--text` (the collector reruns it with `--text`) |

The simulator reads version 5 and version 6 `.traceg` files.

### 4. Collect a trace and a profile

```bash
go run ./nvidia/tracecollector -out nvidia/traces/atax-256 -- \
    nvidia/benchmarks/bin/polybench-atax -x 256 -y 256

go run ./nvidia/tracecollector -out nvidia/traces/pathfinder-64x1024 -- \
    nvidia/benchmarks/bin/rodinia-pathfinder -rows 64 -cols 1024
```

`tracecollector` runs the program twice, because the NVBit tracer and Nsight
Compute cannot instrument the same run:

1. **Trace**: under the NVBit tracer, producing the trace that step 5
   simulates.
2. **Profile**: under Nsight Compute (`ncu`), measuring every kernel on the
   real GPU. This is the hardware number to compare the simulation with.

At the end it prints two reports, for example (atax on an H100):

```
# Trace Info
Status:         OK
Directory:      /home/enze/workspace/mgpusim/nvidia/traces/atax-256
Kernels:        2
Memory copies:  2
Trace size:     1.1156 MB (kernelslist.g and 2 .traceg files)
Simulate with:  ./nvidia/nvidia -trace-dir /home/enze/workspace/mgpusim/nvidia/traces/atax-256

# Profile Info
Status:         OK
Tool:           /usr/local/cuda-12.5/bin/ncu (Version 2025.2.1.0 (build 35987062) (public-release))
Kernels:        2
  ID   Kernel               Cycles    Duration (us)
  0    atax_kernel1          84814           77.504
  1    atax_kernel2         153149          139.904
Total cycles:   237963
Total duration: 217.408 us
Average clock:  1095 MHz (total cycles / total duration)
Saved:          /home/enze/workspace/mgpusim/nvidia/traces/atax-256/profile.txt, /home/enze/workspace/mgpusim/nvidia/traces/atax-256/profile_ncu.csv
```

- **Trace size** is the size of everything the simulator reads
  (`kernelslist.g` plus the `.traceg` files), with 1 MB = 10^6 bytes.
- **Cycles** come straight from ncu, as in mnt-collector: ncu's "Elapsed
  Cycles" (`gpc__cycles_elapsed.max`) for each kernel, so there is no
  conversion through the clock frequency. **Duration** is ncu's
  `gpu__time_duration.sum`. The average clock is only shown for reference.
  ncu only collects these two metrics, which keeps profiling fast.
- ncu locks the GPU clock to its base frequency while it profiles (its
  default `--clock-control base`), so the cycles are stable from run to run
  and the duration reflects the base clock rather than boost.
- The profile report is also saved as `profile.txt` in the output directory,
  and ncu's raw output as `profile_ncu.csv`.

**If profiling is not possible**, the trace is still collected and
`# Profile Info` says why, for example:

```
# Profile Info
Status:         Not available: ncu not found in PATH, /usr/local/cuda*/bin, or /opt/nvidia/nsight-compute; install Nsight Compute or pass -ncu <path>
```

Common reasons are: ncu is not installed or not found (pass `-ncu <path>`
or set `NCU`), no GPU is visible (checked with `nvidia-smi -L`), the user may
not read GPU performance counters (`ERR_NVGPUCTRPERM`, see below), or the
program failed. ncu's output of a failed profile is kept as
`profile_ncu_failed.log`, and it never replaces an earlier good `profile.txt`
or `profile_ncu.csv`.

#### Fixing `ERR_NVGPUCTRPERM` (no permission to read performance counters)

By default the NVIDIA driver lets only administrators read the GPU performance
counters that ncu needs, so profiling fails with:

```
Status:         Not available: ncu: ERR_NVGPUCTRPERM - The user does not have permission to access NVIDIA GPU Performance Counters ...
```

Check the setting; `1` means admins only, `0` means every user may profile:

```bash
grep RmProfilingAdminOnly /proc/driver/nvidia/params
```

To allow every user, reload the NVIDIA driver once with
`NVreg_RestrictProfilingToAdminUsers=0`. This needs `sudo` only for these two
commands; tracecollector itself then runs as a normal user.

```bash
sudo modprobe -r nvidia_uvm nvidia_drm nvidia_modeset nvidia
sudo modprobe nvidia NVreg_RestrictProfilingToAdminUsers=0
```

- Unloading the driver stops every program that uses the GPU, so check
  `nvidia-smi` first and make sure nobody else is using the server's GPUs.
- If `modprobe -r` says a module is in use, stop what holds it (the GPU
  programs listed by `nvidia-smi`, and `sudo systemctl stop nvidia-persistenced`
  if that service runs), then try again.
- The setting lasts until the next reboot. To keep it, add it to the driver
  options and rebuild the initramfs (Ubuntu/Debian shown), then reboot:

  ```bash
  echo 'options nvidia NVreg_RestrictProfilingToAdminUsers=0' | sudo tee /etc/modprobe.d/nvidia-profiling.conf
  sudo update-initramfs -u
  ```

Run `grep RmProfilingAdminOnly /proc/driver/nvidia/params` again; it should
print `0`. Then add the profile to an existing trace without tracing again:

```bash
go run ./nvidia/tracecollector -profile-only -out nvidia/traces/atax-256 -- \
    nvidia/benchmarks/bin/polybench-atax -x 256 -y 256
```

#### Collecting only the trace or only the profile

| Flag | Effect |
| --- | --- |
| `-trace-only` | Only collect the trace. `-tracer` and `-processor` are required. |
| `-profile-only` | Only profile. The tracer is not needed, and `-out` may already hold a trace, so you can add a profile to a trace collected earlier. Exits with status 1 if the profile is not available. |

#### What the trace run does

Without `-profile-only`, the `-out` directory must not exist yet or be empty;
delete it to retry. The trace run follows the steps of mnt-collector:

1. Runs the program with `LD_PRELOAD` set to the tracer and
   `TRACES_FOLDER=<out>`.
2. Moves the files out of `<out>/traces/`.
3. Finds the raw kernel list (`kernelslist*`), decompresses every
   `kernel-*.trace.xz` it lists, and writes `kernelslist_processed` without the
   `.xz` suffixes.
4. Runs the post-processor on `kernelslist_processed`, which writes
   `kernelslist.g` and one `.traceg` per kernel.
5. Removes the raw traces and `kernelslist_processed` (pass `-keep-raw` to keep
   them).

The result is a directory like:

```
nvidia/traces/atax-256/
├── kernelslist.g                    # memory copies and kernel launches, in order
├── kernel-1-ctx_0x....traceg
├── kernel-2-ctx_0x....traceg
├── kernelslist_ctx_0x...            # the tracer's own list (kept for reference)
├── stats_ctx_0x...                  # the tracer's per-kernel statistics
├── profile.txt                      # the "# Profile Info" report
└── profile_ncu.csv                  # ncu's raw output
```

Flags can replace the environment variables: `-tracer <tracer_tool.so>`,
`-processor <post-traces-processing>`, and `-ncu <ncu>`. Pick the GPU with
`CUDA_VISIBLE_DEVICES`. Traces grow with the number of dynamic instructions,
so start with small inputs. Traces are portable: you can copy the directory to
another machine to simulate it.

### 5. Simulate the trace

```bash
./nvidia/nvidia -trace-dir nvidia/traces/atax-256
./nvidia/nvidia -trace-dir nvidia/traces/atax-256 -device A100
```

The output ends with the simulated time:

```
device:          H100 @ 1755 MHz
kernels:         2
warps:           16
instructions:    ...
simulated time:  ... us
simulated cycles: ...
```

Flags:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-trace-dir` | (required) | Directory that contains `kernelslist.g`. |
| `-device` | `H100` | `H100` or `A100`. |
| `-progress` | off | Print the progress (simulated time, finished kernels, running warps, issued instructions, memory requests in flight) to stderr at this interval, e.g. `-progress 5s`. |
| `-trace-vis` | `false` | Record a Daisen visualization trace into an SQLite database. This makes the simulation much slower. |
| `-output` | `akita_sim_<id>` | Name of that database (only with `-trace-vis`). |

Opcodes the pipeline table does not know are mapped to the most similar known
opcode, or to a one-cycle default, and reported once on stderr.

## Development

```bash
go test ./nvidia/...
golangci-lint run ./nvidia/...
```

The tests use a two-thread-block excerpt of a vectorAdd trace in
`trace/testdata/vectoradd`, so they need no GPU.
