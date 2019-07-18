all: bindata.go

bindata.go: kernels.hsaco
	esc -o $@ -pkg bicg -private $^

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/bicg.cl
	clang-ocl -mcpu=fiji -o $@ $^
