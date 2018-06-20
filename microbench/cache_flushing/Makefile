all: kernel

CLANG=/opt/rocm/hcc/bin/clang

kernel: kernel.o dispatch.o
	g++ kernel.o dispatch.o -g -o kernel -L /opt/rocm/lib -l hsa-runtime64

dispatch.o: ../common/dispatch.cpp
	g++ ../common/dispatch.cpp -std=c++14 -g -c -o dispatch.o -I ../common -I /opt/rocm/include/hsa

kernel.o: kernel.cpp
	g++ kernel.cpp -std=c++14 -c -g -o kernel.o -I /opt/rocm/include/hsa -I ../common

kernels.hsaco: kernels.s
	$(CLANG) -x assembler -target amdgcn--amdhsa -mcpu=fiji -c -o kernels.o kernels.s
	$(CLANG) -target amdgcn--amdhsa kernels.o -o kernels.hsaco

clean:
	rm *.o kernel *.bc
