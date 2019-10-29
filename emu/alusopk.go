package emu

import (
	"log"
)

func (u *ALUImpl) runSOPK(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSMOVKI32(state)
	case 3:
		u.runSCMPKLGI32(state)
	case 15:
		u.runSMULKI32(state)
	default:
		log.Panicf("Opcode %d for SOPK format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runSMOVKI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	sp.DST = uint64(imm)
}

func (u *ALUImpl) runSCMPKLGI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	if asInt16(uint16(sp.DST)) != imm {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSMULKI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	dst := asInt32(uint32(sp.DST))

	sp.DST = int64ToBits(int64(int32(imm) * dst))
}
