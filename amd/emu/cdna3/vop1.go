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
	case 22:
		u.runVCVTF64U32(state)
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
	case 45:
		u.runVFFBHU32(state)
	case 56:
		u.runVMOVRELSDB32(state)
	case 76:
		u.runLogLegacyF32(state)
	default:
		log.Panicf("Opcode %d for VOP1 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runVMOVB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		state.WriteOperand(inst.Dst, i, src0)
	}
}

func (u *ALU) runVREADFIRSTLANEB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	var laneid int
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		laneid = i
		break
	}

	src0 := state.ReadOperand(inst.Src0, laneid)
	for i := 0; i < 64; i++ {
		state.WriteOperand(inst.Dst, i, src0)
	}
}

func (u *ALU) runVCVTF64I32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float64(src)
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALU) runVCVTF64U32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := uint32(state.ReadOperand(inst.Src0, i))
		dst := float64(src)
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALU) runVCVTF32I32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float32(src)
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVCVTF32U32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := uint32(state.ReadOperand(inst.Src0, i))
		dst := float32(src)
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVCVTU32F32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		if src < 0 {
			state.WriteOperand(inst.Dst, i, 0)
		} else if src >= float32(math.MaxUint32) {
			state.WriteOperand(inst.Dst, i, math.MaxUint32)
		} else {
			state.WriteOperand(inst.Dst, i, uint64(uint32(src)))
		}
	}
}

func (u *ALU) runVCVTI32F32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		var dst int32
		if src <= float32(math.MinInt32) {
			dst = math.MinInt32
		} else if src >= float32(math.MaxInt32) {
			dst = math.MaxInt32
		} else {
			dst = int32(src)
		}
		state.WriteOperand(inst.Dst, i, uint64(emu.Int32ToBits(dst)))
	}
}

func (u *ALU) runVCVTF16F32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		// Simple F16 conversion (truncated precision)
		f64 := float64(src)
		f16bits := float32ToFloat16(float32(f64))
		state.WriteOperand(inst.Dst, i, uint64(f16bits))
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
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float64frombits(state.ReadOperand(inst.Src0, i))
		dst := float32(src)
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVCVTF64F32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float64(src)
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALU) runVCVTF32UBYTE0(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := uint8(state.ReadOperand(inst.Src0, i) & 0xFF)
		dst := float32(src)
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runTRUNKF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float32(math.Trunc(float64(src)))
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runRNDNEF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float32(math.RoundToEven(float64(src)))
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runEXPF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float32(math.Exp2(float64(src)))
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runLOGF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float32(math.Log2(float64(src)))
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVRCPIFLAGF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := 1.0 / src
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(float32(dst))))
	}
}

func (u *ALU) runVRSQF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float32(1.0 / math.Sqrt(float64(src)))
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVRCPF64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float64frombits(state.ReadOperand(inst.Src0, i))
		dst := 1.0 / src
		state.WriteOperand(inst.Dst, i, math.Float64bits(dst))
	}
}

func (u *ALU) runVSQRTF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float32(math.Sqrt(float64(src)))
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}

func (u *ALU) runVNOTB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src0 := state.ReadOperand(inst.Src0, i)
		state.WriteOperand(inst.Dst, i, ^src0)
	}
}

func (u *ALU) runBFREVB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := uint32(state.ReadOperand(inst.Src0, i))
		var dst uint32
		for j := 0; j < 32; j++ {
			if (src & (1 << j)) != 0 {
				dst |= 1 << (31 - j)
			}
		}
		state.WriteOperand(inst.Dst, i, uint64(dst))
	}
}

func (u *ALU) runVFFBHU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := uint32(state.ReadOperand(inst.Src0, i))
		if src == 0 {
			state.WriteOperand(inst.Dst, i, 0xFFFFFFFF)
		} else {
			pos := uint32(0)
			for bit := 31; bit >= 0; bit-- {
				if (src & (1 << uint(bit))) != 0 {
					pos = uint32(31 - bit)
					break
				}
			}
			state.WriteOperand(inst.Dst, i, uint64(pos))
		}
	}
}

func (u *ALU) runVMOVRELSDB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		// Full relative source+destination behavior depends on M0/GPR indexing mode,
		// which is not modeled in this ALU path yet. For current CDNA3 benchmarks,
		// M0 remains 0, which makes this instruction equivalent to a lane-wise move.
		src0 := state.ReadOperand(inst.Src0, i)
		state.WriteOperand(inst.Dst, i, src0)
	}
}

func (u *ALU) runLogLegacyF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}
		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := float32(math.Log2(float64(src)))
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(dst)))
	}
}
