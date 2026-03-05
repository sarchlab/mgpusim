package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

func (u *ALU) runSMEM(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSLOADDWORD(state)
	case 1:
		u.runSLOADDWORDX2(state)
	case 2:
		u.runSLOADDWORDX4(state)
	case 3:
		u.runSLOADDWORDX8(state)
	case 4:
		u.runSLOADDWORDX16(state)
	default:
		log.Panicf("Opcode %d for SMEM format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSLOADDWORD(state emu.InstEmuState) {
	inst := state.Inst()
	base := state.ReadOperand(inst.Base, 0)
	offset := state.ReadOperand(inst.Offset, 0)
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, base+offset, 4)
	state.WriteOperandBytes(inst.Data, 0, buf)
}

func (u *ALU) runSLOADDWORDX2(state emu.InstEmuState) {
	inst := state.Inst()
	base := state.ReadOperand(inst.Base, 0)
	offset := state.ReadOperand(inst.Offset, 0)
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, base+offset, 8)
	state.WriteOperandBytes(inst.Data, 0, buf)
}

func (u *ALU) runSLOADDWORDX4(state emu.InstEmuState) {
	inst := state.Inst()
	base := state.ReadOperand(inst.Base, 0)
	offset := state.ReadOperand(inst.Offset, 0)
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, base+offset, 16)
	state.WriteOperandBytes(inst.Data, 0, buf)
}

func (u *ALU) runSLOADDWORDX8(state emu.InstEmuState) {
	inst := state.Inst()
	base := state.ReadOperand(inst.Base, 0)
	offset := state.ReadOperand(inst.Offset, 0)
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, base+offset, 32)
	state.WriteOperandBytes(inst.Data, 0, buf)
}

func (u *ALU) runSLOADDWORDX16(state emu.InstEmuState) {
	inst := state.Inst()
	base := state.ReadOperand(inst.Base, 0)
	offset := state.ReadOperand(inst.Offset, 0)
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, base+offset, 64)
	state.WriteOperandBytes(inst.Data, 0, buf)
}

//nolint:gocyclo
func (u *ALU) runSOPP(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0: // S_NOP
		// Do nothing
	case 2: // S_CBRANCH
		u.runSCBRANCH(state)
	case 4: // S_CBRANCH_SCC0
		u.runSCBRANCHSCC0(state)
	case 5: // S_CBRANCH_SCC1
		u.runSCBRANCHSCC1(state)
	case 6: // S_CBRANCH_VCCZ
		u.runSCBRANCHVCCZ(state)
	case 7: // S_CBRANCH_VCCNZ
		u.runSCBRANCHVCCNZ(state)
	case 8: // S_CBRANCH_EXECZ
		u.runSCBRANCHEXECZ(state)
	case 9: // S_CBRANCH_EXECNZ
		u.runSCBRANCHEXECNZ(state)
	case 12: // S_WAITCNT
		// Do nothing
	default:
		log.Panicf("Opcode %d for SOPP format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSCBRANCH(state emu.InstEmuState) {
	inst := state.Inst()
	immRaw := state.ReadOperand(inst.SImm16, 0)
	imm := emu.AsInt16(uint16(immRaw & 0xffff))
	pc := state.PC()
	state.SetPC(uint64(int64(pc) + int64(imm)*4))
}

func (u *ALU) runSCBRANCHSCC0(state emu.InstEmuState) {
	inst := state.Inst()
	immRaw := state.ReadOperand(inst.SImm16, 0)
	imm := emu.AsInt16(uint16(immRaw & 0xffff))
	if state.SCC() == 0 {
		pc := state.PC()
		state.SetPC(uint64(int64(pc) + int64(imm)*4))
	}
}

func (u *ALU) runSCBRANCHSCC1(state emu.InstEmuState) {
	inst := state.Inst()
	immRaw := state.ReadOperand(inst.SImm16, 0)
	imm := emu.AsInt16(uint16(immRaw & 0xffff))
	if state.SCC() == 1 {
		pc := state.PC()
		state.SetPC(uint64(int64(pc) + int64(imm)*4))
	}
}

func (u *ALU) runSCBRANCHVCCZ(state emu.InstEmuState) {
	inst := state.Inst()
	immRaw := state.ReadOperand(inst.SImm16, 0)
	imm := emu.AsInt16(uint16(immRaw & 0xffff))
	if state.VCC() == 0 {
		pc := state.PC()
		state.SetPC(uint64(int64(pc) + int64(imm)*4))
	}
}

func (u *ALU) runSCBRANCHVCCNZ(state emu.InstEmuState) {
	inst := state.Inst()
	immRaw := state.ReadOperand(inst.SImm16, 0)
	imm := emu.AsInt16(uint16(immRaw & 0xffff))
	if state.VCC() != 0 {
		pc := state.PC()
		state.SetPC(uint64(int64(pc) + int64(imm)*4))
	}
}

func (u *ALU) runSCBRANCHEXECZ(state emu.InstEmuState) {
	inst := state.Inst()
	immRaw := state.ReadOperand(inst.SImm16, 0)
	imm := emu.AsInt16(uint16(immRaw & 0xffff))
	if state.EXEC() == 0 {
		pc := state.PC()
		state.SetPC(uint64(int64(pc) + int64(imm)*4))
	}
}

func (u *ALU) runSCBRANCHEXECNZ(state emu.InstEmuState) {
	inst := state.Inst()
	immRaw := state.ReadOperand(inst.SImm16, 0)
	imm := emu.AsInt16(uint16(immRaw & 0xffff))
	if state.EXEC() != 0 {
		pc := state.PC()
		state.SetPC(uint64(int64(pc) + int64(imm)*4))
	}
}
