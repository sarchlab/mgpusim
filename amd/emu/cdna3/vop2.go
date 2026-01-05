package cdna3

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

//nolint:gocyclo,funlen
func (u *ALU) runVOP2(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runVCNDMASKB32(state)
	case 1:
		u.runVADDF32(state)
	case 2:
		u.runVSUBF32(state)
	case 3:
		u.runVSUBREVF32(state)
	case 5:
		u.runVMULF32(state)
	case 6:
		u.runVMULI32I24(state)
	case 8:
		u.runVMULU32U24(state)
	case 10:
		u.runVMINF32(state)
	case 11:
		u.runVMAXF32(state)
	case 12:
		u.runVMINI32(state)
	case 13:
		u.runVMAXI32(state)
	case 14:
		u.runVMINU32(state)
	case 15:
		u.runVMAXU32(state)
	case 16:
		u.runVLSHRREVB32(state)
	case 17:
		u.runVASHRREVI32(state)
	case 18:
		u.runVLSHLREVB32(state)
	case 19:
		u.runVANDB32(state)
	case 20:
		u.runVORB32(state)
	case 21:
		u.runVXORB32(state)
	case 22:
		u.runVMACF32(state)
	case 23:
		u.runVMADAKF32(state)
	case 25:
		u.runVADDI32(state) // v_add_u32 with VCC carry out
	case 26:
		u.runVSUBI32(state) // v_sub_u32 with VCC borrow out
	case 27:
		u.runVSUBREVI32(state) // v_subrev_u32 with VCC borrow out
	case 28:
		u.runVADDCU32(state)
	case 29:
		u.runVSUBBU32(state)
	case 30:
		u.runVSUBBREVU32(state)
	case 42:
		u.runVLSHLREVB16(state)
	case 52:
		u.runVADDU32(state)
	default:
		log.Panicf("Opcode %d for VOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runVCNDMASKB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		if (sp.VCC & (1 << i)) > 0 {
			sp.DST[i] = sp.SRC1[i]
		} else {
			sp.DST[i] = sp.SRC0[i]
		}
	}
}

func (u *ALU) runVADDF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := src0 + src1
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVSUBF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := src0 - src1
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVSUBREVF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := src1 - src0
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVMULF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := src0 * src1
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVMULI32I24(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	const signExtMask int32 = -16777216 // 0xFF000000 as signed int32
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := int32(sp.SRC0[i]) & 0x00FFFFFF
		if src0&0x00800000 != 0 {
			src0 |= signExtMask
		}
		src1 := int32(sp.SRC1[i]) & 0x00FFFFFF
		if src1&0x00800000 != 0 {
			src1 |= signExtMask
		}
		sp.DST[i] = uint64(emu.Int32ToBits(src0 * src1))
	}
}

func (u *ALU) runVMULU32U24(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	for i := 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, uint(i)) {
			continue
		}
		src0 := uint32(sp.SRC0[i]) & 0x00FFFFFF
		src1 := uint32(sp.SRC1[i]) & 0x00FFFFFF
		sp.DST[i] = uint64(src0 * src1)
	}
}

func (u *ALU) runVMINF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := float32(math.Min(float64(src0), float64(src1)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVMAXF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := float32(math.Max(float64(src0), float64(src1)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVMINI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	for i := 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, uint(i)) {
			continue
		}
		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))
		if src0 < src1 {
			sp.DST[i] = uint64(emu.Int32ToBits(src0))
		} else {
			sp.DST[i] = uint64(emu.Int32ToBits(src1))
		}
	}
}

func (u *ALU) runVMAXI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	for i := 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, uint(i)) {
			continue
		}
		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))
		if src0 > src1 {
			sp.DST[i] = uint64(emu.Int32ToBits(src0))
		} else {
			sp.DST[i] = uint64(emu.Int32ToBits(src1))
		}
	}
}

func (u *ALU) runVMINU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	for i := 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, uint(i)) {
			continue
		}
		src0 := uint32(sp.SRC0[i])
		src1 := uint32(sp.SRC1[i])
		if src0 < src1 {
			sp.DST[i] = uint64(src0)
		} else {
			sp.DST[i] = uint64(src1)
		}
	}
}

func (u *ALU) runVMAXU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	for i := 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, uint(i)) {
			continue
		}
		src0 := uint32(sp.SRC0[i])
		src1 := uint32(sp.SRC1[i])
		if src0 > src1 {
			sp.DST[i] = uint64(src0)
		} else {
			sp.DST[i] = uint64(src1)
		}
	}
}

func (u *ALU) runVLSHRREVB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint32(sp.SRC0[i]) & 0x1F
		src1 := uint32(sp.SRC1[i])
		sp.DST[i] = uint64(src1 >> src0)
	}
}

func (u *ALU) runVASHRREVI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint32(sp.SRC0[i]) & 0x1F
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))
		sp.DST[i] = uint64(emu.Int32ToBits(src1 >> src0))
	}
}

func (u *ALU) runVLSHLREVB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint32(sp.SRC0[i]) & 0x1F
		src1 := uint32(sp.SRC1[i])
		sp.DST[i] = uint64(src1 << src0)
	}
}

func (u *ALU) runVANDB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		sp.DST[i] = sp.SRC0[i] & sp.SRC1[i]
	}
}

func (u *ALU) runVORB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		sp.DST[i] = sp.SRC0[i] | sp.SRC1[i]
	}
}

func (u *ALU) runVXORB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		sp.DST[i] = sp.SRC0[i] ^ sp.SRC1[i]
	}
}

func (u *ALU) runVMACF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := math.Float32frombits(uint32(sp.DST[i]))
		dst = src0*src1 + dst
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVMADAKF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	k := math.Float32frombits(uint32(sp.LiteralConstant))
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		dst := src0*src1 + k
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALU) runVADDI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	for i := uint(0); i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]
		result := src0 + src1
		sp.DST[i] = result & 0xFFFFFFFF
		if result > 0xFFFFFFFF {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVSUBI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint32(sp.SRC0[i])
		src1 := uint32(sp.SRC1[i])
		result := src0 - src1
		sp.DST[i] = uint64(result)
		if src1 > src0 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVSUBREVI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint32(sp.SRC0[i])
		src1 := uint32(sp.SRC1[i])
		result := src1 - src0
		sp.DST[i] = uint64(result)
		if src0 > src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVADDCU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]
		carry := (sp.VCC >> i) & 1
		result := src0 + src1 + carry
		sp.DST[i] = result & 0xFFFFFFFF
		if result > 0xFFFFFFFF {
			sp.VCC |= (1 << i)
		} else {
			sp.VCC &= ^(uint64(1) << i)
		}
	}
}

func (u *ALU) runVSUBBU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]
		borrow := (sp.VCC >> i) & 1
		result := src0 - src1 - borrow
		sp.DST[i] = result & 0xFFFFFFFF
		if src1+borrow > src0 {
			sp.VCC |= (1 << i)
		} else {
			sp.VCC &= ^(uint64(1) << i)
		}
	}
}

func (u *ALU) runVSUBBREVU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]
		borrow := (sp.VCC >> i) & 1
		result := src1 - src0 - borrow
		sp.DST[i] = result & 0xFFFFFFFF
		if src0+borrow > src1 {
			sp.VCC |= (1 << i)
		} else {
			sp.VCC &= ^(uint64(1) << i)
		}
	}
}

func (u *ALU) runVLSHLREVB16(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint16(sp.SRC0[i]) & 0x0F
		src1 := uint16(sp.SRC1[i])
		sp.DST[i] = uint64(src1 << src0)
	}
}

// runVADDU32 implements v_add_u32 (simple unsigned 32-bit add, no carry output)
func (u *ALU) runVADDU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOP2()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := uint32(sp.SRC0[i])
		src1 := uint32(sp.SRC1[i])
		sp.DST[i] = uint64(src0 + src1)
	}
}
