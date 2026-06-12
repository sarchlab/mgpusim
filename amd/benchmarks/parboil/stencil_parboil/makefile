all: kernels.disasm kernels.hsaco

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/stencil_parboil.cpp
	cd native && $(MAKE)
	cp native/stencil_parboil.hsaco $@
