package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

//nolint:gocyclo
func (u *ALU) runSOP1(state emu.InstEmuState) {
	inst := state.Inst()
	// Opcodes match the GCN3 decode table numbering (used by the shared decoder)
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

func (u *ALU) runSMOVB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	state.WriteOperand(inst.Dst, 0, src0)
}

func (u *ALU) runSMOVB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	state.WriteOperand(inst.Dst, 0, src0)
}

func (u *ALU) runSNOTU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	dst := ^src0
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	}
}

func (u *ALU) runSBREVB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src := uint32(src0)
	var dst uint32
	for i := 0; i < 32; i++ {
		if (src & (1 << i)) != 0 {
			dst |= 1 << (31 - i)
		}
	}
	state.WriteOperand(inst.Dst, 0, uint64(dst))
}

func (u *ALU) runSGETPCB64(state emu.InstEmuState) {
	inst := state.Inst()
	pc := state.PC()
	state.WriteOperand(inst.Dst, 0, pc)
}

func (u *ALU) runSANDSAVEEXECB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 & exec
	state.SetEXEC(exec)
	if exec == 0 {
		state.SetSCC(0)
	} else {
		state.SetSCC(1)
	}
}

func (u *ALU) runSORSAVEEXECB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 | exec
	state.SetEXEC(exec)
	if exec == 0 {
		state.SetSCC(0)
	} else {
		state.SetSCC(1)
	}
}

func (u *ALU) runSXORSAVEEXECB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 ^ exec
	state.SetEXEC(exec)
	if exec == 0 {
		state.SetSCC(0)
	} else {
		state.SetSCC(1)
	}
}

func (u *ALU) runSANDN2SAVEEXECB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 & ^exec
	state.SetEXEC(exec)
	if exec == 0 {
		state.SetSCC(0)
	} else {
		state.SetSCC(1)
	}
}

func (u *ALU) runSORN2SAVEEXECB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = src0 | ^exec
	state.SetEXEC(exec)
	if exec == 0 {
		state.SetSCC(0)
	} else {
		state.SetSCC(1)
	}
}

func (u *ALU) runSNANDSAVEEXECB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = ^(src0 & exec)
	state.SetEXEC(exec)
	if exec == 0 {
		state.SetSCC(0)
	} else {
		state.SetSCC(1)
	}
}

func (u *ALU) runSNORSAVEEXECB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = ^(src0 | exec)
	state.SetEXEC(exec)
	if exec == 0 {
		state.SetSCC(0)
	} else {
		state.SetSCC(1)
	}
}

func (u *ALU) runSNXORSAVEEXECB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	exec := state.EXEC()
	state.WriteOperand(inst.Dst, 0, exec)
	exec = ^(src0 ^ exec)
	state.SetEXEC(exec)
	if exec == 0 {
		state.SetSCC(0)
	} else {
		state.SetSCC(1)
	}
}

func (u *ALU) runSABSI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src := emu.AsInt32(uint32(src0))
	if src < 0 {
		state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(-src)))
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(src)))
		state.SetSCC(0)
	}
}


