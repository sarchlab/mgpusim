package emu

import "log"

func (u *ALUImpl) runSOP1(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSMOVB32(state)
	case 1:
		u.runSMOVB64(state)
	case 28:
		u.runSGETPCB64(state)
	case 32:
		u.runSANDSAVEEXECB64(state)
	default:
		log.Panicf("Opcode %d for SOP1 format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runSMOVB32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.SRC0
}

func (u *ALUImpl) runSMOVB64(state InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.SRC0
}

func (u *ALUImpl) runSGETPCB64(state InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.PC + 4
}
func (u *ALUImpl) runSANDSAVEEXECB64(state InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = sp.SRC0 & sp.EXEC
	if sp.EXEC != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}
