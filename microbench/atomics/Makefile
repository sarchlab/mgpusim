all: kernel

kernel: kernel.o dispatch.o
	g++ dispatch.o kernel.o -o kernel -L /opt/rocm/lib -l hsa-runtime64

kernel.o: kernel.cpp
	g++ kernel.cpp -std=c++14 -c -o kernel.o -I /opt/rocm/include/hsa -I ../common

dispatch.o: ../common/dispatch.cpp
	g++ ../common/dispatch.cpp -c -o dispatch.o -I /opt/rocm/include/hsa

