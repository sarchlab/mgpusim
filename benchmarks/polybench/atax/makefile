all: bindata.go

bindata.go: kernels.hsaco
	esc -o $@ -pkg atax -private $^

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/atax.cl
	clang-ocl -mcpu=fiji -o $@ $^
