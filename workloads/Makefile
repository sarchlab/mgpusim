# workloads/Makefile -- Top-level build orchestrator
#
# Usage:
#   make all                  # Build all benchmarks for detected platform
#   make all PLATFORM=hip     # Force HIP builds
#   make all PLATFORM=cuda    # Force CUDA builds
#   make suite-polybench      # Build only polybench suite
#   make clean                # Clean all builds

# --- Platform Detection ---
PLATFORM ?= $(shell \
    if command -v hipcc >/dev/null 2>&1; then echo hip; \
    elif command -v nvcc >/dev/null 2>&1; then echo cuda; \
    else echo none; fi)

ifeq ($(PLATFORM),none)
$(error No GPU compiler found. Install CUDA toolkit or ROCm.)
endif

# --- GPU Architecture Detection ---
ifeq ($(PLATFORM),hip)
  COMPILER := hipcc
  GPU_ARCH ?= $(shell rocminfo 2>/dev/null | grep -m1 'Name:.*gfx' | awk '{print $$2}' || echo gfx942)
  CFLAGS ?= -O2 --offload-arch=$(GPU_ARCH)
else
  COMPILER := nvcc
  GPU_ARCH ?= $(shell nvidia-smi --query-gpu=compute_cap --format=csv,noheader 2>/dev/null | head -1 | tr -d '.' | sed 's/^/sm_/' || echo sm_80)
  CFLAGS ?= -O2 -arch=$(GPU_ARCH)
endif

COMMON_DIR := $(CURDIR)/common

# Suites with directories present in the tree
SUITES := $(shell find . -maxdepth 1 -mindepth 1 -type d \
    ! -name common ! -name scripts ! -name results ! -name '.*' \
    -exec basename {} \; | sort)

export PLATFORM COMPILER GPU_ARCH CFLAGS COMMON_DIR

# --- Build Targets ---
.PHONY: all clean

all:
	@for s in $(SUITES); do \
	    if [ -d "$$s" ] && [ -f "$$s/Makefile" ]; then \
	        echo "=== Building suite: $$s ($(PLATFORM)) ==="; \
	        $(MAKE) -C $$s all; \
	    elif [ -d "$$s" ]; then \
	        echo "[SKIP] Suite $$s -- no Makefile"; \
	    else \
	        echo "[SKIP] Suite $$s -- directory not found"; \
	    fi; \
	done

suite-%:
	@if [ -d "$*" ] && [ -f "$*/Makefile" ]; then \
	    echo "=== Building suite: $* ($(PLATFORM)) ==="; \
	    $(MAKE) -C $* all; \
	elif [ -d "$*" ]; then \
	    echo "[SKIP] Suite $* -- no Makefile"; \
	else \
	    echo "[SKIP] Suite $* -- directory not found"; \
	fi

clean:
	@for s in $(SUITES); do \
	    if [ -f "$$s/Makefile" ]; then \
	        $(MAKE) -C $$s clean; \
	    fi; \
	done

info:
	@echo "Platform: $(PLATFORM)"
	@echo "Compiler: $(COMPILER)"
	@echo "GPU arch: $(GPU_ARCH)"
	@echo "CFLAGS:   $(CFLAGS)"
	@echo "Suites:   $(SUITES)"
