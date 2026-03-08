package emu

import "log"

//nolint:gocyclo
func (u *ALUImpl) runSOP1(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSMOVB32(state)
	case 1:
		u.runSMOVB64(state)
	case 4:
		u.runSNOTU32(state)
	case 8:
		u.runSBREVB32(state)
	case 28:
		u.runSGETPCB64(state)
	case 32:
		u.runSANDSAVEEXECB64(state)
	case 33:
		u.runSORSAVEEXECB64(state)
	case 34:
		u.runSXORSAVEEXECB64(state)
	case 35:
		u.runSANDN2SAVEEXECB64(state)
	case 36:
		u.runSORN2SAVEEXECB64(state)
	case 37:
		u.runSNANDSAVEEXECB64(state)
	case 38:
		u.runSNORSAVEEXECB64(state)
	case 39:
		u.runSNXORSAVEEXECB64(state)
	case 48:
		u.runSABSI32(state)
	default:
		log.Panicf("Opcode %d for SOP1 format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runSMOVB32(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	state.WriteOperand(inst.Dst, 0, src0)
}

func (u *ALUImpl) runSMOVB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	state.WriteOperand(inst.Dst, 0, src0)
}

func (u *ALUImpl) runSNOTU32(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	dst := ^src0
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	}
}

func (u *ALUImpl) runSBREVB32(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	dst := uint32(0)
	for i := 0; i < 32; i++ {
		bit := uint32(1 << (31 - i))
		bit = uint32(src0) & bit
		bit = bit >> (31 - i)
		bit = bit << i
		dst = dst | bit
	}
	state.WriteOperand(inst.Dst, 0, uint64(dst))
}

func (u *ALUImpl) runSGETPCB64(state InstEmuState) {
	inst := state.Inst()
	pc := state.PC()
	state.WriteOperand(inst.Dst, 0, pc+4)
}

func (u *ALUImpl) runSANDSAVEEXECB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 & exec
	state.SetEXEC(exec)
	if exec != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALUImpl) runSORSAVEEXECB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 | exec
	state.SetEXEC(exec)
	if exec != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALUImpl) runSXORSAVEEXECB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 ^ exec
	state.SetEXEC(exec)
	if exec != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALUImpl) runSANDN2SAVEEXECB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 & (^exec)
	state.SetEXEC(exec)
	if exec != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALUImpl) runSORN2SAVEEXECB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 | (^exec)
	state.SetEXEC(exec)
	if exec != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALUImpl) runSNANDSAVEEXECB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = ^(src0 & exec)
	state.SetEXEC(exec)
	if exec != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALUImpl) runSNORSAVEEXECB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = ^(src0 | exec)
	state.SetEXEC(exec)
	if exec != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALUImpl) runSNXORSAVEEXECB64(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = ^(src0 ^ exec)
	state.SetEXEC(exec)
	if exec != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALUImpl) runSABSI32(state InstEmuState) {
	inst := state.Inst()
	src0 := asInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	var result int32
	if src0 < 0 {
		result = -src0
	} else {
		result = src0
	}
	state.WriteOperand(inst.Dst, 0, uint64(uint32(result)))
	if result != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}
