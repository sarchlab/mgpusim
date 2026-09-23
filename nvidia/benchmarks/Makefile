# Builds CUDA versions of benchmarks whose kernels live in amd/benchmarks.
#
# The kernel sources are shared with the AMD benchmarks: each host program in
# this directory #includes the HIP kernel file from $(KERNEL_ROOT), and
# include/hip/hip_runtime.h maps the few HIP names those kernels use to CUDA.
#
# Usage (on a machine with the CUDA toolkit):
#   make                 # builds bin/atax and bin/pathfinder
#   make ARCH=sm_80      # target an A100 instead of an H100
#   make KERNEL_ROOT=../../benchmarks   # if the shared kernels move

NVCC        ?= nvcc
ARCH        ?= sm_90
KERNEL_ROOT ?= ../../amd/benchmarks
NVCCFLAGS   ?= -O3 -lineinfo

BIN := bin
INCLUDES := -Iinclude -I$(KERNEL_ROOT)

BENCHMARKS := atax pathfinder

atax_SRC       := polybench/atax/atax.cu
atax_KERNEL    := $(KERNEL_ROOT)/polybench/atax/native/atax.cpp
pathfinder_SRC    := rodinia/pathfinder/pathfinder.cu
pathfinder_KERNEL := $(KERNEL_ROOT)/rodinia/pathfinder/native/rodinia_pathfinder.cpp

.PHONY: all clean
all: $(addprefix $(BIN)/,$(BENCHMARKS))

.SECONDEXPANSION:
$(BIN)/%: $$($$*_SRC) $$($$*_KERNEL) include/hip/hip_runtime.h include/check.h
	@mkdir -p $(BIN)
	$(NVCC) $(NVCCFLAGS) -arch=$(ARCH) $(INCLUDES) -o $@ $<

clean:
	rm -rf $(BIN)
