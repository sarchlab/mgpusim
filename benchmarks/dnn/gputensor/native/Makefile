all: transpose rotate dilate im2col sum_one_axis gemm

transpose: transpose.c
	gcc -g $^ -L/opt/rocm/opencl/lib/ -lOpenCL -o $@

rotate: rotate.c
	gcc -g $^ -L/opt/rocm/opencl/lib/ -lOpenCL -o $@

dilate: dilate.c
	gcc -g $^ -L/opt/rocm/opencl/lib/ -lOpenCL -o $@

im2col: im2col.c
	gcc -g $^ -L/opt/rocm/opencl/lib/ -lOpenCL -o $@

sum_one_axis: sum_one_axis.c
	gcc -g $^ -L/opt/rocm/opencl/lib/ -lOpenCL -o $@

gemm: gemm.c
	gcc -g $^ -L/opt/rocm/opencl/lib/ -lOpenCL -o $@
