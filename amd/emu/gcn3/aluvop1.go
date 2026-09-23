package gcn3

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
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

		src0 := state.ReadOperand(inst.Src0, i)
		dst := math.Float64bits(float64(int32(src0)))
		state.WriteOperand(inst.Dst, i, dst)
	}
}

func (u *ALU) runVCVTF32I32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := state.ReadOperand(inst.Src0, i)
		dst := uint64(math.Float32bits(float32(int32(src0))))
		state.WriteOperand(inst.Dst, i, dst)
	}
}

func (u *ALU) runVCVTF32U32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := state.ReadOperand(inst.Src0, i)
		dst := uint64(math.Float32bits(float32(uint32(src0))))
		state.WriteOperand(inst.Dst, i, dst)
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

		state.WriteOperand(inst.Dst, i, dst)
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

		state.WriteOperand(inst.Dst, i, dst)
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

func (u *ALU) runVRCPIFLAGF32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src := math.Float32frombits(uint32(state.ReadOperand(inst.Src0, i)))
		dst := 1 / src
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

		src := uint32(state.ReadOperand(inst.Src0, i))
		dst := ^src
		state.WriteOperand(inst.Dst, i, uint64(dst))
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
		dst := uint32(0)
		for j := 0; j < 32; j++ {
			bit := uint32(1 << (31 - j))
			bit = src & bit
			bit = bit >> (31 - j)
			bit = bit << j
			dst = dst | bit
		}
		state.WriteOperand(inst.Dst, i, uint64(dst))
	}
}

func (u *ALU) runVCVTF32UBYTE0(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := state.ReadOperand(inst.Src0, i)
		state.WriteOperand(inst.Dst, i, uint64(math.Float32bits(
			float32((uint32(src0)<<24)>>24))))
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
		state.WriteOperand(inst.Dst, i, math.Float64bits(float64(src)))
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
		dst := float64(1.0) / src
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

func (u *ALU) runVCVTF16F32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		src0 := state.ReadOperand(inst.Src0, i)
		sign := uint16((uint32(src0) >> 31) & 0x1)
		exp := (uint32(src0) >> 23) & 0xff
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
		state.WriteOperand(inst.Dst, i, uint64(f16))
	}
}

func (u *ALU) runLogLegacyF32(state emu.InstEmuState) {
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
