# polybench/Makefile -- Build all PolyBench GPU benchmarks
#
# Usage:
#   make all                       # build all for detected platform
#   make all PLATFORM=hip          # force HIP
#   make all PLATFORM=cuda         # force CUDA
#   make bench-atax                # build only atax
#   make clean                     # clean all builds

PLATFORM ?= hip
COMPILER ?= hipcc
GPU_ARCH ?= gfx942
CFLAGS ?= -O2 --offload-arch=$(GPU_ARCH)
COMMON_DIR ?= $(CURDIR)/../common

export PLATFORM COMPILER GPU_ARCH CFLAGS COMMON_DIR

# Discover benchmark directories (those with cuda/ or hip/ subdirs)
BENCHMARKS := $(shell find . -maxdepth 1 -mindepth 1 -type d \
    -exec basename {} \; | sort)

.PHONY: all clean $(addprefix bench-,$(BENCHMARKS))

all:
	@for b in $(BENCHMARKS); do \
	    if [ -d "$$b/$(PLATFORM)" ] && [ -f "$$b/$(PLATFORM)/Makefile" ]; then \
	        echo "[BUILD] polybench/$$b ($(PLATFORM))"; \
	        $(MAKE) -C $$b/$(PLATFORM) all \
	            || echo "[WARN] polybench/$$b build failed"; \
	    elif [ -d "$$b/$(PLATFORM)" ]; then \
	        echo "[SKIP] polybench/$$b -- no Makefile in $(PLATFORM)/"; \
	    fi; \
	done

bench-%:
	@if [ -d "$*/$(PLATFORM)" ] && [ -f "$*/$(PLATFORM)/Makefile" ]; then \
	    echo "[BUILD] polybench/$* ($(PLATFORM))"; \
	    $(MAKE) -C $*/$(PLATFORM) all; \
	else \
	    echo "[SKIP] polybench/$* -- no Makefile in $(PLATFORM)/"; \
	fi

clean:
	@for b in $(BENCHMARKS); do \
	    if [ -d "$$b/$(PLATFORM)" ] && [ -f "$$b/$(PLATFORM)/Makefile" ]; then \
	        $(MAKE) -C $$b/$(PLATFORM) clean; \
	    fi; \
	done
