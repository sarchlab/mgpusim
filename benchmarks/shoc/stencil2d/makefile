all: kernels.disasm kernels.hsaco

kernels.disasm: stencil2d.cl
	clang-ocl -mcpu=fiji -S -o $@ stencil2d.cl

kernels.hsaco: stencil2d.cl
	clang-ocl -mcpu=fiji -o $@ stencil2d.cl
