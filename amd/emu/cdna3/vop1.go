package cdna3

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

//nolint:gocyclo,funlen
func (u *ALU) runVOP1(state emu.InstEmuState) {
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

func (u *ALU) runVMOVB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		sp.DST[i] = sp.SRC0[i]
	}
}

func (u *ALU) runVREADFIRSTLANEB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	var laneid uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		laneid = i
		break
	}
	for i = 0; i < 64; i++ {
		sp.DST[i] = sp.SRC0[laneid]
	}
}

func (u *ALU) runVCVTF64I32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := emu.AsInt32(uint32(sp.SRC0[i]))
		dst := float64(src)
		sp.DST[i] = math.Float64bits(dst)
	}
}

func (u *ALU) runVCVTF32I32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := emu.AsInt32(uint32(sp.SRC0[i]))
		dst := float32(src)
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVCVTF32U32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := uint32(sp.SRC0[i])
		dst := float32(src)
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVCVTU32F32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		if src < 0 {
			sp.DST[i] = 0
		} else if src >= float32(math.MaxUint32) {
			sp.DST[i] = math.MaxUint32
		} else {
			sp.DST[i] = uint64(uint32(src))
		}
	}
}

func (u *ALU) runVCVTI32F32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		var dst int32
		if src <= float32(math.MinInt32) {
			dst = math.MinInt32
		} else if src >= float32(math.MaxInt32) {
			dst = math.MaxInt32
		} else {
			dst = int32(src)
		}
		sp.DST[i] = uint64(emu.Int32ToBits(dst))
	}
}

func (u *ALU) runVCVTF16F32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		// Simple F16 conversion (truncated precision)
		f64 := float64(src)
		f16bits := float32ToFloat16(float32(f64))
		sp.DST[i] = uint64(f16bits)
	}
}

func float32ToFloat16(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := (bits >> 31) & 1
	exp := (bits >> 23) & 0xFF
	frac := bits & 0x7FFFFF

	var f16exp, f16frac uint16

	if exp == 0 {
		// Zero or denormal
		f16exp = 0
		f16frac = 0
	} else if exp == 0xFF {
		// Inf or NaN
		f16exp = 31
		if frac != 0 {
			f16frac = 1 // NaN
		} else {
			f16frac = 0 // Inf
		}
	} else {
		// Normal number
		newExp := int(exp) - 127 + 15
		if newExp >= 31 {
			f16exp = 31
			f16frac = 0 // Inf
		} else if newExp <= 0 {
			f16exp = 0
			f16frac = 0 // Zero
		} else {
			f16exp = uint16(newExp)
			f16frac = uint16(frac >> 13)
		}
	}

	return (uint16(sign) << 15) | (f16exp << 10) | f16frac
}

func (u *ALU) runVCVTF32F64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float64frombits(sp.SRC0[i])
		dst := float32(src)
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVCVTF64F32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float64(src)
		sp.DST[i] = math.Float64bits(dst)
	}
}

func (u *ALU) runVCVTF32UBYTE0(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := uint8(sp.SRC0[i] & 0xFF)
		dst := float32(src)
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runTRUNKF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Trunc(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runRNDNEF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.RoundToEven(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runEXPF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Exp2(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runLOGF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Log2(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVRCPIFLAGF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := 1.0 / src
		sp.DST[i] = uint64(math.Float32bits(float32(dst)))
	}
}

func (u *ALU) runVRSQF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(1.0 / math.Sqrt(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVRCPF64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float64frombits(sp.SRC0[i])
		dst := 1.0 / src
		sp.DST[i] = math.Float64bits(dst)
	}
}

func (u *ALU) runVSQRTF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Sqrt(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVNOTB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		sp.DST[i] = ^sp.SRC0[i]
	}
}

func (u *ALU) runBFREVB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := uint32(sp.SRC0[i])
		var dst uint32
		for j := 0; j < 32; j++ {
			if (src & (1 << j)) != 0 {
				dst |= 1 << (31 - j)
			}
		}
		sp.DST[i] = uint64(dst)
	}
}

func (u *ALU) runLogLegacyF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Log2(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}
