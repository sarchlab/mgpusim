package emu

import (
	"log"
)

//nolint:gocyclo,funlen
func (u *ALUImpl) runSOPC(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSCMPEQU32(state)
	case 1:
		u.runSCMPLGU32(state)
	case 2:
		u.runSCMPGTI32(state)
	case 3:
		u.runSCMPGEI32(state)
	case 4:
		u.runSCMPLTI32(state)
	case 6:
		u.runSCMPEQU32(state)
	case 7:
		u.runSCMPLGU32(state)
	case 8:
		u.runSCMPGTU32(state)
	case 10:
		u.runSCMPLTU32(state)
	default:
		log.Panicf("Opcode %d for SOPC format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runSCMPGTI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	src0 := asInt32(uint32(sp.SRC0))
	src1 := asInt32(uint32(sp.SRC1))
	if src0 > src1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSCMPLTI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	src0 := asInt32(uint32(sp.SRC0))
	src1 := asInt32(uint32(sp.SRC1))
	if src0 < src1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSCMPGEI32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	src0 := asInt32(uint32(sp.SRC0))
	src1 := asInt32(uint32(sp.SRC1))
	if src0 >= src1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSCMPEQU32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 == sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSCMPLGU32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 != sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSCMPGTU32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 > sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALUImpl) runSCMPLTU32(state InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 < sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}
