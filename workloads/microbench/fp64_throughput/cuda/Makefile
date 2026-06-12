# microbench/fp64_throughput/cuda/Makefile -- CUDA benchmark build
COMPILER ?= nvcc
GPU_ARCH ?= sm_80
CFLAGS ?= -O2 -arch=$(GPU_ARCH)
COMMON_DIR ?= ../../../common

TARGET := fp64_throughput
SOURCES := fp64_throughput.cu

.PHONY: all clean

all: $(TARGET)

$(TARGET): $(SOURCES)
	$(COMPILER) $(CFLAGS) -I$(COMMON_DIR) -o $@ $<

clean:
	rm -f $(TARGET)
