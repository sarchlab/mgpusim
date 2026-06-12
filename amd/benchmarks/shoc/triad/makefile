all: kernels.disasm kernels.hsaco

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/triad.cpp
	cd native && $(MAKE)
	cp native/triad.hsaco $@
