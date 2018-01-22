package emu

import (
	"log"
)

func (u *ALU) runVOP3A(state InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 196: // 0xC4
		u.runVCmpGtI32VOP3a(state)
	case 201: // 0xC9
		u.runVCmpLtU32VOP3a(state)
	case 202: // 0xCA
		u.runVCmpEqU32VOP3a(state)
	case 203: // 0xCB
		u.runVCmpLeU32VOP3a(state)
	case 204: // 0xCC
		u.runVCmpGtU32VOP3a(state)
	case 205: // 0xCD
		u.runVCmpLgU32VOP3a(state)
	case 206: // 0xCE
		u.runVCmpGeU32VOP3a(state)
	case 256:
		u.runVCNDMASKB32VOP3a(state)
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
	inst := state.Inst()

	if inst.Abs != 0 {
		log.Panic("Oprand absolute operation is not supported.")
	}

	if inst.Neg != 0 {
		log.Panic("Oprand negative operation is not supported.")
	}
}

func (u *ALU) vop3aPostprocess(state InstEmuState) {
	inst := state.Inst()

	if inst.Omod != 0 {
		log.Panic("Output modifiers are not supported.")
	}
}

func (u *ALU) runVCmpGtI32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := int32(sp.SRC0[i])
		src1 := int32(sp.SRC1[i])

		if src0 > src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLtU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpEqU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 == src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLeU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 <= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGtU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 > src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpLgU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 != src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCmpGeU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 >= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALU) runVCNDMASKB32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !u.laneMasked(sp.EXEC, i) {
			continue
		}

		if (sp.SRC2[i] & (1 << i)) > 0 {
			sp.DST[i] = sp.SRC1[i]
		} else {
			sp.DST[i] = sp.SRC0[i]
		}

	}

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
