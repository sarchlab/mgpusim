all: kernels.disasm kernels.hsaco

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/2mm.cl
	clang-ocl -mcpu=fiji -o $@ $^
