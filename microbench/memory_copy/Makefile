all: kernel

kernel: kernel.o
	g++ kernel.o -o kernel -L /opt/rocm/lib -l hsa-runtime64

kernel.o: kernel.cpp
	g++ kernel.cpp -std=c++14 -c -o kernel.o -I /opt/rocm/include/hsa -I ../common

clean:
	rm *.o kernel *.bc
