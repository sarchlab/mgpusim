all: kernels.disasm kernels.hsaco

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/gaussian.cpp
	cd native && $(MAKE)
	cp native/gaussian.hsaco $@
