package emu

import "log"

func (u *ALU) runSOP1(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSMOVB32(state)
	case 32:
		u.runSANDSAVEEXECB64(state)
	default:
		log.Panicf("Opcode %d for SOP1 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSMOVB32(state InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.SRC0
}

func (u *ALU) runSANDSAVEEXECB64(state InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = sp.SRC0 & sp.EXEC
	if sp.EXEC != 0 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}
