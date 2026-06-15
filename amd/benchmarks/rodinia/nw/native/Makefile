DOCKER_IMAGE = rocm/dev-ubuntu-24.04:7.1.1
GPU_TARGET = gfx942
SRC = nw.cpp
GPU_OBJ = nw-hip-amdgcn-amd-amdhsa-$(GPU_TARGET).o
HSACO = nw.hsaco
DISASM = nw.disasm

DOCKER_RUN = docker run --rm --platform linux/amd64 -v $(PWD):/work -w /work $(DOCKER_IMAGE)

.PHONY: all clean regen-disasm

all: $(HSACO)

$(HSACO): $(SRC)
	$(DOCKER_RUN) hipcc --save-temps -c --offload-arch=$(GPU_TARGET) $<
	cp $(GPU_OBJ) $(HSACO)

regen-disasm: $(HSACO)
	$(DOCKER_RUN) /opt/rocm/llvm/bin/llvm-objdump --disassemble --mcpu=$(GPU_TARGET) $(HSACO) > $(DISASM)

clean:
	rm -f *.ll *.bc *.s *.o *.cui *.fatbin *.hipfb *.out *.txt *.hipi
