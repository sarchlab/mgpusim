# Builds CUDA versions of the benchmarks whose kernels live in amd/benchmarks.
#
# Every <suite>/<name>/<name>.cu here is a host program that #includes the
# HIP kernel file from $(KERNEL_ROOT)/<suite>/<name>/native/, and
# include/hip/hip_runtime.h maps the HIP names those kernels use to CUDA.
# New benchmarks are picked up automatically.
#
# Usage (on a machine with the CUDA toolkit):
#   make                   # builds bin/<name> for every benchmark
#   make bin/gemm          # builds one benchmark
#   make ARCH=sm_80        # target an A100 instead of an H100
#   make list              # prints the benchmark names
#   make KERNEL_ROOT=../../benchmarks   # if the shared kernels move

NVCC        ?= nvcc
ARCH        ?= sm_90
KERNEL_ROOT ?= ../../amd/benchmarks
NVCCFLAGS   ?= -O3 -lineinfo

BIN      := bin
SOURCES  := $(wildcard polybench/*/*.cu rodinia/*/*.cu)
NAMES    := $(basename $(notdir $(SOURCES)))
HEADERS  := include/hip/hip_runtime.h include/check.h
INCLUDES := -Iinclude -I$(KERNEL_ROOT)

.PHONY: all clean list
all: $(addprefix $(BIN)/,$(NAMES))

list:
	@echo $(NAMES)

# bin/<name> depends on <suite>/<name>/<name>.cu and on the shared kernels.
define BENCHMARK_RULE
$(BIN)/$(basename $(notdir $(1))): $(1) $(wildcard $(KERNEL_ROOT)/$(dir $(1))native/*.cpp) $(HEADERS)
	@mkdir -p $(BIN)
	$$(NVCC) $$(NVCCFLAGS) -arch=$$(ARCH) $$(INCLUDES) -o $$@ $$<
endef
$(foreach src,$(SOURCES),$(eval $(call BENCHMARK_RULE,$(src))))

clean:
	rm -rf $(BIN)
