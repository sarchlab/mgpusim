package emu

import (
	"log"
	"math"

	"gitlab.com/yaotsu/gcn3/insts"
)

func (u *ALUImpl) runSOP2(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSADDU32(state)
	case 2:
		u.runSADDI32(state)
	case 3:
		u.runSSUBI32(state)
	case 4:
		u.runSADDCU32(state)
	case 7:
		u.runSMINU32(state)
	case 12:
		u.runSANDB32(state)
	case 13:
		u.runSANDB64(state)
	case 15:
		u.runSORB64(state)
	case 17:
		u.runSXOR64(state)
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
	case 34:
		u.runSBFMB32(state)
	case 36:
		u.runSMULI32(state)
	default:
		log.Panicf("Opcode %d for SOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runSADDU32(state InstEmuState) {
	sp := state.Scratchpad()

	src0 := insts.BytesToUint32(sp[0:8])
	src1 := insts.BytesToUint32(sp[8:16])

	dst := src0 + src1
	scc := byte(0)
	if src0 > math.MaxUint32-src1 {
		scc = 1
	} else {
		scc = 0
	}

	copy(sp[16:24], insts.Uint32ToBytes(dst))
	sp[24] = scc
}

func (u *ALUImpl) runSADDI32(state InstEmuState) {
	sp := state.Scratchpad()

	src0 := insts.BytesToUint32(sp[0:8])
	src1 := insts.BytesToUint32(sp[8:16])

	dst := src0 + src1
	scc := byte(0)
	if src0 > math.MaxUint32-src1 {
		scc = 1
	} else {
		scc = 0
	}

	copy(sp[16:24], insts.Uint32ToBytes(dst))
	sp[24] = scc
}

func (u *ALUImpl) runSSUBI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	src0 := asInt32(uint32(sp.SRC0))
	src1 := asInt32(uint32(sp.SRC1))
	dst := src0 - src1

	if src1 > 0 && dst > src0 {
		sp.SCC = 1
	} else if src1 < 0 && dst < src0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}

	sp.DST = uint64(int32ToBits(dst))
}

func (u *ALUImpl) runSADDCU32(state InstEmuState) {
	sp := state.Scratchpad()

	src0 := insts.BytesToUint32(sp[0:8])
	src1 := insts.BytesToUint32(sp[8:16])
	scc := sp[24]

	dst := src0 + src1 + uint32(scc)
	if src0 < math.MaxUint32-uint32(scc)-src1 {
		scc = 0
	} else {
		scc = 1
	}

	copy(sp[16:24], insts.Uint32ToBytes(dst))
	sp[24] = scc
}

func (u *ALUImpl) runSMINU32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	if sp.SRC0 < sp.SRC1 {
		sp.DST = sp.SRC0
		sp.SCC = 1
	} else {
		sp.DST = sp.SRC1
	}
}

func (u *ALUImpl) runSANDB32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	sp.DST = sp.SRC0 & sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSANDB64(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	sp.DST = sp.SRC0 & sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSORB64(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 | sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSXOR64(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	sp.DST = sp.SRC0 ^ sp.SRC1
	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSLSHLB32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	src0 := uint32(sp.SRC0)
	src1 := uint8(sp.SRC1)
	dst := src0 << (src1 & 0x1f)

	sp.DST = uint64(dst)

	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSLSHLB64(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	src0 := sp.SRC0
	src1 := uint8(sp.SRC1)
	dst := src0 << (src1 & 0x3f)

	sp.DST = dst

	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSLSHRB32(state InstEmuState) {

	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 >> (sp.SRC1 & 0x1f)

	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSLSHRB64(state InstEmuState) {

	sp := state.Scratchpad().AsSOP2()
	sp.DST = sp.SRC0 >> (sp.SRC1 & 0x3f)

	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSASHRI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	src0 := asInt32(uint32(sp.SRC0))
	src1 := uint8(sp.SRC1)
	dst := src0 >> src1

	sp.DST = uint64(int32ToBits(dst))

	if sp.DST != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSBFMB32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	sp.DST = ((1 << (sp.SRC0 & 0x1f)) - 1) << (sp.SRC1 & 0x1f)
}

func (u *ALUImpl) runSMULI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP2()

	src0 := asInt32(uint32(sp.SRC0))
	src1 := asInt32(uint32(sp.SRC1))
	dst := src0 * src1

	sp.DST = uint64(int32ToBits(dst))
}
