# microbench/l1_cache_bw/hip/Makefile -- HIP benchmark build
COMPILER ?= hipcc
GPU_ARCH ?= gfx942
CFLAGS ?= -O2 --offload-arch=$(GPU_ARCH)
COMMON_DIR ?= ../../../common

TARGET := l1_cache_bw
SOURCES := l1_cache_bw.cpp

.PHONY: all clean

all: $(TARGET)

$(TARGET): $(SOURCES)
	$(COMPILER) $(CFLAGS) -I$(COMMON_DIR) -o $@ $<

clean:
	rm -f $(TARGET)
