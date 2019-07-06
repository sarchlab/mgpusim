all:
	clang-ocl -mcpu=fiji -S -o kernels.disasm native/MatrixTranspose_Kernels.cl
	clang-ocl -mcpu=fiji -o kernels.hsaco native/MatrixTranspose_Kernels.cl
	go generate
