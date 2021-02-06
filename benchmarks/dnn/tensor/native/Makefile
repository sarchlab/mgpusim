all: transpose rotate dilate im2col sum_one_axis

transpose: transpose.c
	gcc -g $^ -L/opt/rocm/lib/ -lOpenCL -o $@

rotate: rotate.c
	gcc -g $^ -L/opt/rocm/lib/ -lOpenCL -o $@

dilate: dilate.c
	gcc -g $^ -L/opt/rocm/lib/ -lOpenCL -o $@

im2col: im2col.c
	gcc -g $^ -L/opt/rocm/lib/ -lOpenCL -o $@

sum_one_axis: sum_one_axis.c
	gcc -g $^ -L/opt/rocm/lib/ -lOpenCL -o $@

