package emu

import (
	"fmt"
	"log"
	"math"
)

//nolint:gocyclo,funlen
func (u *ALUImpl) runVOP1(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 1:
		u.runVMOVB32(state)
	case 2:
		u.runVREADFIRSTLANEB32(state)
	case 4:
		u.runVCVTF64I32(state)
	case 5:
		u.runVCVTF32I32(state)
	case 6:
		u.runVCVTF32U32(state)
	case 7:
		u.runVCVTU32F32(state)
	case 8:
		u.runVCVTI32F32(state)
	case 10:
		u.runVCVTF16F32(state)
	case 15:
		u.runVCVTF32F64(state)
	case 16:
		u.runVCVTF64F32(state)
	case 17:
		u.runVCVTF32UBYTE0(state)
	case 28:
		u.runTRUNKF32(state)
	case 30:
		u.runRNDNEF32(state)
	case 32:
		u.runEXPF32(state)
	case 33:
		u.runLOGF32(state)
	case 34, 35:
		u.runVRCPIFLAGF32(state)
	case 36:
		u.runVRSQF32(state)
	case 37:
		u.runVRCPF64(state)
	case 39:
		u.runVSQRTF32(state)
	case 43:
		u.runVNOTB32(state)
	case 44:
		u.runBFREVB32(state)
	case 76:
		u.runLogLegacyF32(state)

	default:
		log.Panicf("Opcode %d for VOP1 format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runVMOVB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC0[i]
	}
}

func (u *ALUImpl) runVREADFIRSTLANEB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	var laneid uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}
		laneid = i
		break
	}

	for i = 0; i < 64; i++ {
		sp.DST[i] = sp.SRC0[laneid]
	}
}

func (u *ALUImpl) runVCVTF64I32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = math.Float64bits(float64(int32(sp.SRC0[i])))
	}
}

func (u *ALUImpl) runVCVTF32I32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = uint64(math.Float32bits(float32(int32(sp.SRC0[i]))))
	}
}

func (u *ALUImpl) runVCVTF32U32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = uint64(math.Float32bits(float32(uint32(sp.SRC0[i]))))
	}
}

func (u *ALUImpl) runVCVTU32F32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		//src := math.Float32bits(float32(uint32(sp.SRC0[i])))

		var dst uint64
		if math.IsNaN(float64(src)) {
			dst = 0
		} else if src < 0 {
			dst = 0
		} else if uint64(src) > math.MaxUint32 {
			dst = math.MaxUint32
		} else {
			dst = uint64(src)
		}

		sp.DST[i] = dst
	}
}

func (u *ALUImpl) runVCVTI32F32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		var dst uint64
		if math.IsNaN(float64(src)) || math.IsNaN(float64(0-src)) {
			dst = 0
		} else if int32(src) > math.MaxInt32 {
			dst = math.MaxInt32
		} else if int32(src) < (0 - math.MaxInt32) {
			dst = uint64(int32ToBits(0 - math.MaxInt32))
		} else {
			dst = uint64(int32ToBits(int32(src)))
		}

		sp.DST[i] = dst
	}
}

func (u *ALUImpl) runTRUNKF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Trunc(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runRNDNEF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.RoundToEven(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runEXPF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Exp2(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runLOGF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Log2(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runVRSQF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(1.0 / math.Sqrt(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runVRCPIFLAGF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := 1 / src
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runVNOTB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := uint32(sp.SRC0[i])
		dst := ^src
		sp.DST[i] = uint64(dst)
	}
}

func (u *ALUImpl) runBFREVB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := uint32(sp.SRC0[i])
		dst := uint32(0)
		for i := 0; i < 32; i++ {
			bit := uint32(1 << (31 - i))
			bit = src & bit
			bit = bit >> (31 - i)
			bit = bit << i
			dst = dst | bit
		}
		sp.DST[i] = uint64(dst)
	}
}

func (u *ALUImpl) runVCVTF32UBYTE0(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = uint64(math.Float32bits(
			float32((uint32(sp.SRC0[i]) << 24) >> 24)))
	}
}

func (u *ALUImpl) runVCVTF64F32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		sp.DST[i] = math.Float64bits(float64(src))
	}
}

func (u *ALUImpl) runVRCPF64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float64frombits(sp.SRC0[i])
		dst := float64(1.0) / src
		sp.DST[i] = math.Float64bits(dst)
	}
}

func (u *ALUImpl) runVSQRTF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Sqrt(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runVCVTF32F64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float64frombits(sp.SRC0[i])
		dst := float32(src)
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runVCVTF16F32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sign := uint16((uint32(sp.SRC0[i]) >> 31) & 0x1)
		exp := (uint32(sp.SRC0[i]) >> 23) & 0xff
		exp16 := int16(exp) - 127 + 15
		frac := uint16(i>>13) & 0x3ff
		if exp == 0 {
			exp16 = 0
		} else if exp == 0xff {
			exp16 = 0x1f
		} else {
			if exp16 > 0x1e {
				exp16 = 0x1f
				frac = 0
			} else if exp16 < 0x01 {
				exp16 = 0
				frac = 0
			}
		}
		f16 := (sign << 15) | uint16(exp16<<10) | frac
		sp.DST[i] = uint64(f16)
	}
}

func (u *ALUImpl) runLogLegacyF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	fmt.Printf("Print value inst:\n")

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		fmt.Printf("Print value %d\n", sp.SRC0[i])

		sp.DST[i] = sp.SRC0[i]
	}
}
