all: kernel

CLANG=/opt/rocm/hcc/bin/clang

kernel: kernel.o dispatch.o
	g++ dispatch.o kernel.o -o kernel -L /opt/rocm/lib -l hsa-runtime64

kernel.o: kernel.cpp
	g++ kernel.cpp -std=c++14 -c -o kernel.o \
		-I /opt/rocm/include/hsa -I ../../common

dispatch.o: ../../common/dispatch.cpp
	g++ ../../common/dispatch.cpp -c -o dispatch.o \
		-I /opt/rocm/include/hsa

kernels.hsaco: kernels.s
	$(CLANG) -x assembler -target amdgcn--amdhsa -mcpu=gfx803 \
		-c -o kernels.o kernels.s
	$(CLANG) -target amdgcn--amdhsa kernels.o -o kernels.hsaco



clean:
	rm kernel *.o *.bc
