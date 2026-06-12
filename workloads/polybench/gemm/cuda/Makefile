# polybench/gemm/cuda/Makefile -- CUDA benchmark build
COMPILER ?= nvcc
GPU_ARCH ?= sm_80
CFLAGS ?= -O2 -arch=$(GPU_ARCH)
COMMON_DIR ?= ../../../common

TARGET := gemm
SOURCES := gemm.cu

.PHONY: all clean

all: $(TARGET)

$(TARGET): $(SOURCES)
	$(COMPILER) $(CFLAGS) -I$(COMMON_DIR) -o $@ $<

clean:
	rm -f $(TARGET)
