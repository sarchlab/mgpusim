# Include from a native kernel directory after setting SRC, HSACO, and DISASM.
ROCM_PATH ?= /opt/rocm
GFX942_DIR := $(dir $(lastword $(MAKEFILE_LIST)))
GFX942_BUILD := $(GFX942_DIR)build-gfx942.sh
EMBEDDED_HSACO ?= ../kernels_gfx942.hsaco

.PHONY: all clean clean-all regen-disasm update-embedded

all: $(HSACO)

$(HSACO): $(SRC) Makefile $(GFX942_DIR)gfx942.mk $(GFX942_BUILD)
	ROCM_PATH="$(ROCM_PATH)" "$(GFX942_BUILD)" "$<" "$@" $(HIP_FLAGS)

update-embedded: $(HSACO)
	cp "$(HSACO)" "$(EMBEDDED_HSACO)"

regen-disasm: $(HSACO)
	@set -eu; \
	  if [ "$$(cat "$(ROCM_PATH)/.info/version" 2>/dev/null)" != 10.2.0 ]; then \
	    echo "ROCM_PATH must point to a ROCm 10.2.0 installation" >&2; exit 1; \
	  fi; \
	  tmp=$$(mktemp "$(DISASM).XXXXXX"); \
	  trap 'rm -f "$$tmp"' 0; \
	  "$(ROCM_PATH)/lib/llvm/bin/llvm-objdump" --disassemble --mcpu=gfx942 "$(HSACO)" > "$$tmp"; \
	  cat "$$tmp" > "$(DISASM)"

clean:
	rm -f *.ll *.bc *.s *.o *.cui *.fatbin *.hipfb

clean-all: clean
	rm -f "$(HSACO)" "$(DISASM)"
