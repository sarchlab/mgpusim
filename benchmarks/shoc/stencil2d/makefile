all: bindata.go kernels.disasm

bindata.go: kernels.hsaco
	esc -o $@ -pkg stencil2d -private $^

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -disassemble -mcpu=fiji $^ > $@

kernels.hsaco: stencil2d.cl
	clang-ocl -mcpu=fiji -o $@ stencil2d.cl
