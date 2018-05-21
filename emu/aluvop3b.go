package emu

import (
	"log"
)

func (u *ALUImpl) runVOP3B(state InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 284:
		u.runVADDCU32VOP3b(state)
	default:
		log.Panicf("Opcode %d for VOP3b format is not implemented", inst.Opcode)
	}

	u.vop3aPostprocess(state)
}

func (u *ALUImpl) runVADDCU32VOP3b(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3B()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC0[i] + sp.SRC1[i] + ((sp.SRC2[i] & (1 << i)) >> i)
		carry := uint64(0)
		if sp.DST[i] > 0xffffffff {
			carry = 1
		}
		sp.SDST |= carry << i
		sp.DST[i] &= 0xffffffff
	}

}
