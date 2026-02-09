package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

//nolint:gocyclo,funlen
func (u *ALU) runSOP2(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSADDU32(state)
	case 1:
		u.runSSUBU32(state)
	case 2:
		u.runSADDI32(state)
	case 3:
		u.runSSUBI32(state)
	case 4:
		u.runSADDCU32(state)
	case 5:
		u.runSSUBBU32(state)
	case 6:
		u.runSMINI32(state)
	case 7:
		u.runSMINU32(state)
	case 8:
		u.runSMAXI32(state)
	case 9:
		u.runSMAXU32(state)
	case 10:
		u.runSCSELECTB32(state)
	case 12:
		u.runSANDB32(state)
	case 14:
		u.runSANDB64(state)
	case 15:
		u.runSORB64(state)
	case 16:
		u.runSXORB64(state)
	case 18:
		u.runSANDN2B64(state)
	case 28:
		u.runSLSHLB32(state)
	case 29:
		u.runSLSHLB64(state)
	case 30:
		u.runSLSHRB32(state)
	case 31:
		u.runSLSHRB64(state)
	case 32:
		u.runSASHRI32(state)
	case 33:
		u.runSBFMB32(state)
	case 36:
		u.runSMULI32(state)
	case 37:
		u.runSBFEI32(state)
	default:
		log.Panicf("Opcode %d for SOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSADDU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sum := sp.SRC0 + sp.SRC1
	sp.DST = sum & 0xFFFFFFFF
	if sum > 0xFFFFFFFF {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSSUBU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 - sp.SRC1
	if sp.SRC1 > sp.SRC0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSADDI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	sum := int64(src0) + int64(src1)
	sp.DST = uint64(emu.Int32ToBits(int32(sum)))
	if sum > 0x7FFFFFFF || sum < -0x80000000 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSSUBI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	diff := int64(src0) - int64(src1)
	sp.DST = uint64(emu.Int32ToBits(int32(diff)))
	if diff > 0x7FFFFFFF || diff < -0x80000000 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSADDCU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sum := sp.SRC0 + sp.SRC1 + uint64(sp.SCC)
	sp.DST = sum & 0xFFFFFFFF
	if sum > 0xFFFFFFFF {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSSUBBU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 - sp.SRC1 - uint64(sp.SCC)
	if sp.SRC1+uint64(sp.SCC) > sp.SRC0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSMINI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	if src0 < src1 {
		sp.DST = uint64(emu.Int32ToBits(src0))
		sp.SCC = 1
	} else {
		sp.DST = uint64(emu.Int32ToBits(src1))
		sp.SCC = 0
	}
}

func (u *ALU) runSMINU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	if sp.SRC0 < sp.SRC1 {
		sp.DST = sp.SRC0
		sp.SCC = 1
	} else {
		sp.DST = sp.SRC1
		sp.SCC = 0
	}
}

func (u *ALU) runSMAXI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	if src0 > src1 {
		sp.DST = uint64(emu.Int32ToBits(src0))
		sp.SCC = 1
	} else {
		sp.DST = uint64(emu.Int32ToBits(src1))
		sp.SCC = 0
	}
}

func (u *ALU) runSMAXU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	if sp.SRC0 > sp.SRC1 {
		sp.DST = sp.SRC0
		sp.SCC = 1
	} else {
		sp.DST = sp.SRC1
		sp.SCC = 0
	}
}

func (u *ALU) runSCSELECTB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	if sp.SCC == 1 {
		sp.DST = sp.SRC0
	} else {
		sp.DST = sp.SRC1
	}
}

func (u *ALU) runSANDB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 & sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSANDB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 & sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSORB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 | sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSXORB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 ^ sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSANDN2B64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 & ^sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSLSHLB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	shift := sp.SRC1 & 0x1F
	sp.DST = (sp.SRC0 << shift) & 0xFFFFFFFF
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSLSHLB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	shift := sp.SRC1 & 0x3F
	sp.DST = sp.SRC0 << shift
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSLSHRB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	shift := sp.SRC1 & 0x1F
	sp.DST = uint64(uint32(sp.SRC0) >> shift)
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSLSHRB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	shift := sp.SRC1 & 0x3F
	sp.DST = sp.SRC0 >> shift
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSASHRI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	shift := sp.SRC1 & 0x1F
	result := src0 >> shift
	sp.DST = uint64(emu.Int32ToBits(result))
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSBFMB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	// S_BFM_B32: sp.DST = ((1 << sp.SRC0[4:0]) - 1) << sp.SRC1[4:0]
	width := sp.SRC0 & 0x1F
	offset := sp.SRC1 & 0x1F
	mask := ((uint64(1) << width) - 1) << offset
	sp.DST = mask & 0xFFFFFFFF
}

func (u *ALU) runSMULI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	result := src0 * src1
	sp.DST = uint64(emu.Int32ToBits(result))
}

func (u *ALU) runSBFEI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	// S_BFE_I32: Extract bit field with sign extension
	offset := (sp.SRC1) & 0x1F
	width := (sp.SRC1 >> 16) & 0x7F
	if width == 0 {
		sp.DST = 0
	} else {
		extracted := (sp.SRC0 >> offset) & ((1 << width) - 1)
		// Sign extend
		signBit := (extracted >> (width - 1)) & 1
		if signBit == 1 {
			signExt := ^((uint64(1) << width) - 1)
			extracted |= signExt
		}
		sp.DST = extracted
	}
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}
