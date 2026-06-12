all: kernels_gfx942.hsaco

kernels_gfx942.hsaco: kernel.hip
	/opt/rocm/bin/hipcc --genco --targets gfx942 -o $@ $<

clean:
	rm -f kernels_gfx942.hsaco
