# lonestar/bh/hip/Makefile -- HIP benchmark build
COMPILER ?= hipcc
GPU_ARCH ?= gfx942
CFLAGS ?= -O2 --offload-arch=$(GPU_ARCH)
COMMON_DIR ?= ../../../common

TARGET := bh
SOURCES := bh.cpp

.PHONY: all clean

all: $(TARGET)

$(TARGET): $(SOURCES)
	$(COMPILER) $(CFLAGS) -I$(COMMON_DIR) -o $@ $<

clean:
	rm -f $(TARGET)
