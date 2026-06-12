all: kernels.disasm kernels.hsaco

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/tpacf.cpp
	cd native && $(MAKE)
	cp native/tpacf.hsaco $@
