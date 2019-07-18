all: bindata.go

bindata.go: kernels.hsaco
	esc -o $@ -pkg stencil2d -private $^

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/atax.cl
	clang-ocl -mcpu=fiji -o $@ $^
