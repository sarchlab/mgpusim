package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

func (u *ALU) runSOPK(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSMOVKI32(state)
	case 2:
		u.runSCMOVKI32(state)
	case 3:
		u.runSCMPKEQI32(state)
	case 5:
		u.runSCMPKLGI32(state)
	case 15:
		u.runSMULKI32(state)
	default:
		log.Panicf("Opcode %d for SOPK format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSMOVKI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	sp.DST = uint64(emu.Int16ToBits(emu.AsInt16(uint16(sp.IMM))))
}

func (u *ALU) runSCMOVKI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	if sp.SCC == 1 {
		sp.DST = uint64(emu.Int16ToBits(emu.AsInt16(uint16(sp.IMM))))
	}
}

func (u *ALU) runSCMPKEQI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	imm := int32(emu.AsInt16(uint16(sp.IMM)))
	src := emu.AsInt32(uint32(sp.DST))
	if src == imm {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPKLGI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	imm := int32(emu.AsInt16(uint16(sp.IMM)))
	src := emu.AsInt32(uint32(sp.DST))
	if src != imm {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSMULKI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPK()
	imm := int32(emu.AsInt16(uint16(sp.IMM)))
	src := emu.AsInt32(uint32(sp.DST))
	result := src * imm
	sp.DST = uint64(emu.Int32ToBits(result))
}
