# Kernel compilation workflow (pinned clang, Docker-free)

This directory contains a reproducible workflow for compiling the benchmark GPU
kernels to **gfx942 (MI300A / CDNA3)** code objects (`.hsaco`) using a **pinned
clang/LLVM 18 toolchain + libclc** — no ROCm SDK and no Docker required.

It replaces the previous `hipcc`-inside-Docker recipe (still present in each
benchmark's `native/Makefile`) for environments where the
`rocm/dev-ubuntu-24.04` container cannot be pulled.

## Toolchain (locked)

| Tool | Package (Ubuntu 24.04) | Version |
|------|------------------------|---------|
| `clang-18` | `clang-18` | `1:18.1.3-1ubuntu1` |
| `ld.lld-18` | `lld-18` | `1:18.1.3-1ubuntu1` |
| libclc (`/usr/lib/clc/amdgcn--amdhsa.bc`) | `libclc-18` | `1:18.1.3-1ubuntu1` |

```bash
sudo apt-get install -y clang-18 lld-18 libclc-18
```

The version is enforced by `clang_compile_kernel.sh` (it refuses to run on a
non-18.1 clang) so binaries are reproducible across machines and CI.

## Usage

Compile a single OpenCL kernel:

```bash
amd/benchmarks/tools/clang_compile_kernel.sh input.cl output.hsaco gfx942
```

Compile **every** `*.cl` under `amd/benchmarks/` and decode-verify each with
MGPUSim's own disassembler (this is what CI runs — see
`.github/workflows/compile-kernels.yml`):

```bash
amd/benchmarks/tools/compile_all_kernels.sh
```

Per-source compile flags (e.g. the SHOC templated kernels that need
`-DT2=float2`) live in a sidecar file next to the source, named
`<source>.cl.clflags`.

## How it works

```
clang-18 -x cl -cl-std=CL2.0 --target=amdgcn-amd-amdhsa -mcpu=gfx942 \
  -nogpulib \
  -Xclang -finclude-default-header \
  -Xclang -mlink-builtin-bitcode -Xclang /usr/lib/clc/amdgcn--amdhsa.bc \
  -O2 -fno-slp-vectorize -fno-vectorize \
  -mllvm -amdgpu-load-store-vectorizer=false \
  input.cl -o output.hsaco
```

The `--save-temps` `*.o` trick from the hipcc recipe is unnecessary here: with
`--target=amdgcn-amd-amdhsa`, clang links (via `ld.lld`) directly to a loadable
HSACO. The emitted code object is **COV5** and its kernarg layout matches the
hipcc one exactly (verified with `llvm-readelf --notes`): explicit args, then
`hidden_block_count_*`, `hidden_group_size_*` (offset 44), `hidden_remainder_*`,
`hidden_global_offset_*` — `kernarg_segment_size = 288` for a 5-arg kernel. This
is why the existing `CDNA3*Args` Go structs are directly reusable.

## Writing kernels for MGPUSim's CDNA3 emulator

The functional emulator (`amd/emu/cdna3/`) implements a subset of the gfx942
ISA. When authoring OpenCL kernels for it:

- **Index from hardware IDs, not `get_global_id`.** libclc lowers
  `get_global_id`/`get_local_size` to a read of the AQL dispatch packet via
  `dispatch_ptr`, which the emulator does not map. Instead compute the index
  from `get_group_id` (hardware SGPR) and `get_local_id` (hardware VGPR) with a
  fixed block size, and tag the kernel with `reqd_work_group_size`:

  ```c
  #define BLOCK 256
  __attribute__((reqd_work_group_size(BLOCK, 1, 1)))
  __kernel void k(__global float *out, int n) {
      int i = get_group_id(0) * BLOCK + get_local_id(0);
      if (i < n) { /* ... */ }
  }
  ```
  Launch with a local size equal to `BLOCK`.

- Stay within the emulated instruction subset (no atomics, cross-lane
  shuffles, packed-FP16, or MFMA — see `amd/emu/cdna3/`).

## Status / known limitations

- ✅ All 28 OpenCL sources under `amd/benchmarks/` **compile** and are
  **decoded/parsed** by MGPUSim's disassembler with this toolchain.
- ⚠️ Producing a kernel that the emulator *executes correctly* additionally
  requires following the authoring rules above. clang's default code generation
  differs from hipcc's (dispatch-packet reads for work-group size; scratch
  spills at low optimization levels) in ways the current CDNA3 emulator/driver
  does not yet handle. See the parent task notes for details on the remaining
  emulator/driver work.
