package emu

import "log"

func (u *ALU) runVOP3A(state InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 202: // 0xCA
		u.runVCmpEqU32(state)
	case 645:
		u.runVMULLOU32(state)
	case 646:
		u.runVMULHIU32(state)
	case 655:
		u.runVLSHLREVB64(state)
	case 657:
		u.runVASHRREVI64(state)
	default:
		log.Panicf("Opcode %d for VOP3a format is not implemented", inst.Opcode)
	}

	u.vop3aPostprocess(state)
}

func (u *ALU) vop3aPreprocess(state InstEmuState) {
}

func (u *ALU) vop3aPostprocess(state InstEmuState) {
}

func (u *ALU) runVMULLOU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = (sp.SRC0[i] * sp.SRC1[i])
	}
}

func (u *ALU) runVMULHIU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = (sp.SRC0[i] * sp.SRC1[i]) >> 32
	}

}

func (u *ALU) runVLSHLREVB64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC1[i] << sp.SRC0[i]
	}
}

func (u *ALU) runVASHRREVI64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = int64ToBits(asInt64(sp.SRC1[i]) >> sp.SRC0[i])
	}
}
