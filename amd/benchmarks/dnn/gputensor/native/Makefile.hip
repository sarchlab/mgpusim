# Build the seven gfx942 objects embedded by the Go tensor package.
ROCM_PATH ?= /opt/rocm
GFX942_BUILD := ../../../build-gfx942.sh
KERNELS := operator repeat im2col maxpooling avgpooling gemm cross_entropy
HSACOS := $(addprefix ../,$(addsuffix _gfx942.hsaco,$(KERNELS)))

.PHONY: all update-embedded clean clean-all

all: $(HSACOS)

update-embedded: all

../%_gfx942.hsaco: %_hip.cpp Makefile.hip $(GFX942_BUILD)
	ROCM_PATH="$(ROCM_PATH)" "$(GFX942_BUILD)" "$<" "$@"

clean:
	rm -f *.ll *.bc *.s *.o *.cui *.fatbin *.hipfb

clean-all: clean
	rm -f $(HSACOS)
