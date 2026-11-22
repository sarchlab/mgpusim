# Building ROCm 10.2 kernels for MI300X calibration

The migrated native benchmark Makefiles include `amd/benchmarks/gfx942.mk`.
They use the AMD ROCm 10.2.0 `gfx942` toolchain. The validated AMD Clang 24.0.0git
revision is `bdc35841c97ff51400273b91d7bc05c96e19c2d9`, also used for the MI300X
reference collection.

Install the [ROCm 10.2.0 gfx942 distribution](https://rocm.docs.amd.com/en/docs-10.2.0/install/rocm.html)
on Linux or extract the official `therock-dist-linux-gfx94X-dcgpu-10.2.0.tar.gz`
archive. The compiler needs the usual Linux C++ development headers. To rebuild
one native object and update the copy embedded by its Go sample:

```bash
make -C amd/benchmarks/parboil/sgemm/native ROCM_PATH=/opt/rocm/core-10.2 update-embedded
```

Use `all` (the default) when building only the native object, and `regen-disasm`
to inspect a native object. To force a rebuild, pass `-B` to make.

On macOS, run the Linux toolchain in a container. With the distribution extracted
to a local directory containing `.info/version` and `lib/llvm/bin/clang++`:

```bash
ROCM_DIST=/path/to/extracted/rocm-10.2.0
HOST_IMAGE=rocm/dev-ubuntu-24.04@sha256:bdc8e61026cbb844ede93d44d2c50055f51ebb2041906b60182bf3bee3139054
docker run --rm --platform linux/amd64 \
  -v "$PWD:/work" -v "$ROCM_DIST:/rocm:ro" -w /work \
  -e ROCM_PATH=/rocm -e LD_LIBRARY_PATH=/rocm/lib:/rocm/lib/llvm/lib \
  "$HOST_IMAGE" \
  make -C amd/benchmarks/parboil/sgemm/native update-embedded
```

The pinned ROCm 7.2.4 container provides Linux C++ development headers. The recipe invokes the
compiler and bundler from the mounted 10.2.0 distribution by absolute path.
The compiler produces a device-only bundle. The shared builder unbundles the
linked `gfx942` ELF and validates its target before replacing the output file.
