all: bindata.go

bindata.go: kernels.hsaco
	esc -o $@ -pkg bfs -private $^

kernels.disasm: kernels.hsaco
	/opt/rocm/hcc/bin/llvm-objdump -mcpu=fiji -disassemble $^ > $@

kernels.hsaco: native/kernels.cl
	clang-ocl -mcpu=fiji -o $@ $^
