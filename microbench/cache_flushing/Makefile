all: kernel

CLANG=/opt/rocm/hcc/bin/clang

kernel: kernel.o
	g++ kernel.o -g -o kernel -L /opt/rocm/lib -l hsa-runtime64

kernel.o: kernel.cpp
	g++ kernel.cpp -std=c++14 -c -g -o kernel.o -I /opt/rocm/include/hsa -I ../common

kernels.hsaco: kernels.s
	$(CLANG) -x assembler -target amdgcn--amdhsa -mcpu=fiji -c -o kernels.o kernels.s
	$(CLANG) -target amdgcn--amdhsa kernels.o -o kernels.hsaco

clean:
	rm *.o kernel *.bc
