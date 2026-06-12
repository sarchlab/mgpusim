all: kernels.disasm kernels.hsaco

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/spmv_parboil.cpp
	cd native && $(MAKE)
	cp native/spmv_parboil.hsaco $@
