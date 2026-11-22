SRC = parboil_lbm.cpp
HSACO = parboil_lbm.hsaco
DISASM = parboil_lbm.disasm
# Use the reciprocal instruction for the BGK update. Without fast math, the
# compiler can lower the division to a sequence with unsupported CDNA3 ALU
# operations. Verification allows the reciprocal's small precision difference.
HIP_FLAGS = -ffast-math

include ../../../gfx942.mk
