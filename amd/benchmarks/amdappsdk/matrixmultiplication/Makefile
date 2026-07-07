# Build the GCN3 (gfx803) OpenCL kernel binary consumed by MGPUSim.
#
# MGPUSim's timing/emulation models the AMD GCN3 ISA (gfx803), so this kernel
# must be compiled with a toolchain that still targets gfx803. gfx803 support
# was removed from ROCm after ~4.x, so use the ROCm 3.8 OpenCL offline compiler.
#
# The result (kernels.hsaco) is checked into git and embedded via //go:embed.
#
# Usage (with Docker + a ROCm 3.8 image):
#   make            # rebuild kernels.hsaco
# Or run the clang command below directly on a machine with ROCm 3.8 installed.

GPU_TARGET   = gfx803
SRC          = MatrixMultiplication_Kernels.cl
HSACO        = kernels.hsaco

# A ROCm 3.8-era image that still ships the gfx803 offline OpenCL compiler.
# Override if your local image/tag differs, e.g. `make DOCKER_IMAGE=rocm/rocm-terminal:3.8`.
DOCKER_IMAGE = rocm/dev-ubuntu-18.04:3.8
DOCKER_RUN   = docker run --rm --platform linux/amd64 -v $(PWD):/work -w /work $(DOCKER_IMAGE)
CLANG        = /opt/rocm/llvm/bin/clang

# OpenCL 1.2 offline compile to a gfx803 code object.
# -finclude-default-header pulls in the OpenCL builtin declarations.
CLFLAGS = -x cl -cl-std=CL1.2 -Xclang -finclude-default-header \
          -target amdgcn-amd-amdhsa -mcpu=$(GPU_TARGET)

.PHONY: all clean

all: $(HSACO)

$(HSACO): $(SRC)
	$(DOCKER_RUN) $(CLANG) $(CLFLAGS) $(SRC) -o $(HSACO)

# If you have ROCm 3.8 installed locally (no Docker), compile directly:
local: $(SRC)
	$(CLANG) $(CLFLAGS) $(SRC) -o $(HSACO)

clean:
	@echo "$(HSACO) is checked into git and preserved; remove manually if intended."
