all:
	clang -x assembler -target amdgcn--amdhsa -mcpu=fiji -c -o kernel.o kernel.s
	clang -target amdgcn--amdhsa kernel.o -o kernel.hsaco

clean:
	rm *.o *.hsaco

