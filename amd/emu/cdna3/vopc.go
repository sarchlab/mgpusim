package cdna3

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

//nolint:gocyclo,funlen
func (u *ALU) runVOPC(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	// f32 comparisons (0x40-0x4f range)
	case 0x41:
		u.runVCmpLtF32(state)
	case 0x42:
		u.runVCmpEqF32(state)
	case 0x43:
		u.runVCmpLeF32(state)
	case 0x44:
		u.runVCmpGtF32(state)
	case 0x45:
		u.runVCmpLgF32(state)
	case 0x46:
		u.runVCmpGeF32(state)
	// i32 comparisons (0xc0-0xc7 range)
	case 0xc1:
		u.runVCmpLtI32(state)
	case 0xc3:
		u.runVCmpLeI32(state)
	case 0xc4:
		u.runVCmpGtI32(state)
	case 0xc5:
		u.runVCmpLgI32(state)
	case 0xc6:
		u.runVCmpGeI32(state)
	// u32 comparisons (0xc8-0xcf range)
	case 0xc9:
		u.runVCmpLtU32(state)
	case 0xca:
		u.runVCmpEqU32(state)
	case 0xcb:
		u.runVCmpLeU32(state)
	case 0xcc:
		u.runVCmpGtU32(state)
	case 0xcd:
		u.runVCmpNeU32(state)
	case 0xce:
		u.runVCmpGeU32(state)
	// u64 comparisons (0xe8-0xef range)
	case 0xe8:
		u.runVCmpFU64(state)
	case 0xe9:
		u.runVCmpLtU64(state)
	case 0xea:
		u.runVCmpEqU64(state)
	case 0xeb:
		u.runVCmpLeU64(state)
	case 0xec:
		u.runVCmpGtU64(state)
	case 0xed:
		u.runVCmpLgU64(state)
	case 0xee:
		u.runVCmpGeU64(state)
	case 0xef:
		u.runVCmpTruU64(state)
	default:
		log.Panicf("Opcode %d for VOPC format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runVCmpLtF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 < src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpEqF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 == src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLeF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 <= src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGtF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 > src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLgF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 != src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGeF32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := math.Float32frombits(uint32(sp.SRC0[i]))
		src1 := math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 >= src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLtI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))
		if src0 < src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLeI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))
		if src0 <= src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGtI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))
		if src0 > src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLgI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))
		if src0 != src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGeI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		src0 := emu.AsInt32(uint32(sp.SRC0[i]))
		src1 := emu.AsInt32(uint32(sp.SRC1[i]))
		if src0 >= src1 {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLtU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(sp.EXEC, i) {
			continue
		}
		if sp.SRC0[i] < sp.SRC1[i] {
			sp.VCC |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpEqU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] == sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpLeU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] <= sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpGtU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] > sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpNeU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] != sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpGeU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] >= sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpFU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			// Always false
			sp.VCC &= ^(uint64(1) << i)
		}
	}
}

func (u *ALU) runVCmpLtU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] < sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpEqU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] == sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpLeU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] <= sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpGtU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] > sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpLgU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] != sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpGeU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			if sp.SRC0[i] >= sp.SRC1[i] {
				sp.VCC |= (1 << i)
			}
		}
	}
}

func (u *ALU) runVCmpTruU64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsVOPC()
	var i uint
	for i = 0; i < 64; i++ {
		if emu.LaneMasked(sp.EXEC, i) {
			// Always true
			sp.VCC |= (1 << i)
		}
	}
}
