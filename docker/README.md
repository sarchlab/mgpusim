# Docker Images for MGPUSim

This directory contains Dockerfiles for MGPUSim toolchain environments.

---

## `Dockerfile.rocm` — ROCm HSACO Compilation Environment

### What it contains

| Component | Version / Notes |
|-----------|----------------|
| Base OS | Ubuntu 22.04 LTS |
| ROCm | 6.2 (pinned) |
| Clang/LLVM | Ships with ROCm 6.2 (`/opt/rocm/llvm/bin/clang`) — AMDGPU backend included |
| hipcc | Included via ROCm 6.2 |
| llvm-dis, llvm-objdump | Included via ROCm 6.2 |
| Target GPU | gfx942 (AMD Instinct MI300X / MI300A) |

### Purpose

Provides a **locked, reproducible** compilation environment for turning
`.hip` kernel sources into HSACO ELF objects (`.hsaco`) targeting gfx942.
Using a pinned Docker image prevents silent breakage if the host clang
version changes on the CI runner.

The canonical compilation command used inside this image is:

```bash
clang \
  -target amdgcn-amd-amdhsa \
  -mcpu=gfx942 \
  -x c++ \
  -fvisibility=default \
  kernel.hip \
  -o kernel.hsaco
```

---

## Building the image

```bash
# From the repo root:
docker build \
  -f docker/Dockerfile.rocm \
  -t sarchlab/mgpusim-rocm:6.2 \
  .
```

---

## Running the image

### Interactive shell

```bash
docker run --rm -it sarchlab/mgpusim-rocm:6.2 bash
```

### Compile a single kernel

```bash
docker run --rm \
  -v "$(pwd)/amd/benchmarks/rodinia/bfs:/work" \
  sarchlab/mgpusim-rocm:6.2 \
  clang \
    -target amdgcn-amd-amdhsa \
    -mcpu=gfx942 \
    -x c++ \
    -fvisibility=default \
    /work/kernel.hip \
    -o /work/kernel.hsaco
```

### Compile all benchmarks (batch script pattern)

```bash
for d in amd/benchmarks/*/*; do
  [ -f "$d/kernel.hip" ] || continue
  docker run --rm \
    -v "$(pwd)/$d:/work" \
    sarchlab/mgpusim-rocm:6.2 \
    clang \
      -target amdgcn-amd-amdhsa \
      -mcpu=gfx942 \
      -x c++ \
      -fvisibility=default \
      /work/kernel.hip \
      -o /work/kernels_gfx942.hsaco
done
```

---

## Docker Hub publishing instructions

**Suggested image name:** `sarchlab/mgpusim-rocm`  
**Suggested tags:** `:6.2`, `:6.2-gfx942`, `:latest`

### Prerequisites

1. A Docker Hub account with push access to the `sarchlab` organization.
2. Docker CLI installed and logged in:

```bash
docker login
```

### Step-by-step

```bash
# 1. Build the image
docker build \
  -f docker/Dockerfile.rocm \
  -t sarchlab/mgpusim-rocm:6.2 \
  -t sarchlab/mgpusim-rocm:6.2-gfx942 \
  -t sarchlab/mgpusim-rocm:latest \
  .

# 2. (Optional) Verify the image
docker run --rm sarchlab/mgpusim-rocm:6.2 clang --version

# 3. Push all tags
docker push sarchlab/mgpusim-rocm:6.2
docker push sarchlab/mgpusim-rocm:6.2-gfx942
docker push sarchlab/mgpusim-rocm:latest
```

### Multi-arch build (optional, for aarch64 CI runners)

If the CI runner is `aarch64` but you want to publish an `amd64` image
(or both), use `buildx`:

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -f docker/Dockerfile.rocm \
  -t sarchlab/mgpusim-rocm:6.2 \
  --push \
  .
```

> **Note:** The ROCm base image `rocm/dev-ubuntu-22.04:6.2-complete` is
> only published for `linux/amd64`. For `aarch64` CI runners that only
> cross-compile to AMDGCN (no actual AMD GPU needed), you may need a
> different base — e.g., an Ubuntu 22.04 image with LLVM built from
> source to include the AMDGPU backend. See the compile_hsaco.yml
> workflow for the current native-clang approach used on aarch64.

---

## Updating the ROCm version

1. Change the `FROM` line in `Dockerfile.rocm` to the desired tag (e.g.,
   `rocm/dev-ubuntu-22.04:6.4-complete`).
2. Update the `LABEL rocm.version` accordingly.
3. Rebuild and push with the new tag (e.g., `:6.4`).
4. Update `compile_hsaco.yml` if it references the image tag.

Available ROCm base images: https://hub.docker.com/r/rocm/dev-ubuntu-22.04/tags
