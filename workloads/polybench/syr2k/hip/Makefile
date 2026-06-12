# polybench/syr2k/hip/Makefile -- HIP benchmark build
COMPILER ?= hipcc
GPU_ARCH ?= gfx942
CFLAGS ?= -O2 --offload-arch=$(GPU_ARCH)
COMMON_DIR ?= ../../../common

TARGET := syr2k
SOURCES := syr2k.cpp

.PHONY: all clean

all: $(TARGET)

$(TARGET): $(SOURCES)
	$(COMPILER) $(CFLAGS) -I$(COMMON_DIR) -o $@ $<

clean:
	rm -f $(TARGET)
