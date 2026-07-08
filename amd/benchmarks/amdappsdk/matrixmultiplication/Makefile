# Build the GCN3 (gfx803) OpenCL kernel binary consumed by MGPUSim.
#
# MGPUSim's timing/emulation models the AMD GCN3 ISA (gfx803), so this kernel
# must be compiled with a toolchain that still targets gfx803. gfx803 was
# dropped from ROCm's default device list years ago, but recent ROCm images
# still ship the gfx803 OpenCL device libraries (oclc_isa_version_803.bc) and
# their clang still accepts -mcpu=gfx803, so we can build with a current image.
#
# The result (kernels.hsaco) is a Code Object V5 binary: MGPUSim's loader
# parses the .kd descriptor (kernel entry offset, kernarg size, register
# counts) and the sample's COV5KernelArgs struct supplies the COV5
# hidden-argument block (block counts, group sizes, remainders, grid dims).
# It is checked into git and embedded via //go:embed.
#
# Usage (with Docker):
#   make            # rebuild kernels.hsaco
# Or run the clang command below directly on a machine with ROCm installed.

GPU_TARGET   = gfx803
SRC          = MatrixMultiplication_Kernels.cl
HSACO        = kernels.hsaco

# A recent ROCm image that still ships the gfx803 OpenCL device libraries.
# Override if your local image/tag differs.
DOCKER_IMAGE = rocm/dev-ubuntu-24.04:7.2.4
DOCKER_RUN   = docker run --rm --platform linux/amd64 -v $(PWD):/work -w /work $(DOCKER_IMAGE)
CLANG        = /opt/rocm/llvm/bin/clang

# OpenCL 1.2 offline compile to a gfx803 code object.
# -finclude-default-header pulls in the OpenCL builtin declarations.
# --rocm-path lets clang find and auto-link the gfx803 device bitcode
# (get_global_id, barrier, etc.).
CLFLAGS = -x cl -cl-std=CL1.2 -Xclang -finclude-default-header \
          -target amdgcn-amd-amdhsa -mcpu=$(GPU_TARGET) --rocm-path=/opt/rocm

.PHONY: all clean

all: $(HSACO)

$(HSACO): $(SRC)
	$(DOCKER_RUN) $(CLANG) $(CLFLAGS) $(SRC) -o $(HSACO)

# If you have ROCm installed locally (no Docker), compile directly:
local: $(SRC)
	$(CLANG) $(CLFLAGS) $(SRC) -o $(HSACO)

clean:
	@echo "$(HSACO) is checked into git and preserved; remove manually if intended."
