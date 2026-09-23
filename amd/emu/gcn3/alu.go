// Package gcn3 provides the GCN3 (gfx803) ALU implementation.
package gcn3

import (
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// ALU is the GCN3 (gfx803) ALU implementation. It is where the instructions
// get executed.
type ALU struct {
	storageAccessor emu.StorageAccessor
	lds             []byte
}

// NewALU creates a new GCN3 ALU with a storage as a dependency.
func NewALU(storageAccessor emu.StorageAccessor) *ALU {
	alu := new(ALU)
	alu.storageAccessor = storageAccessor
	return alu
}

// ArchName returns the architecture name.
func (u *ALU) ArchName() string {
	return "GCN3"
}

// SetLDS assigns the LDS storage to be used in the following instructions.
func (u *ALU) SetLDS(lds []byte) {
	u.lds = lds
}

// LDS returns lds
func (u *ALU) LDS() []byte {
	return u.lds
}

// Run executes the instruction in the scatchpad of the InstEmuState
//
//nolint:gocyclo
func (u *ALU) Run(state emu.InstEmuState) {
	inst := state.Inst()
	// fmt.Printf("%s\n", insts.NewInstPrinter(nil).Print(inst))

	switch inst.FormatType {
	case insts.SOP1:
		u.runSOP1(state)
	case insts.SOP2:
		u.runSOP2(state)
	case insts.SOPC:
		u.runSOPC(state)
	case insts.SMEM:
		u.runSMEM(state)
	case insts.VOP1:
		u.runVOP1(state)
	case insts.VOP2:
		u.runVOP2(state)
	case insts.VOP3a:
		u.runVOP3A(state)
	case insts.VOP3b:
		u.runVOP3B(state)
	case insts.VOPC:
		u.runVOPC(state)
	case insts.FLAT:
		u.runFlat(state)
	case insts.SOPP:
		u.runSOPP(state)
	case insts.SOPK:
		u.runSOPK(state)
	case insts.DS:
		u.runDS(state)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}

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

func (u *ALU) sdwaSrcSelect(src uint32, sel insts.SDWASelect) uint32 {
	switch sel {
	case insts.SDWASelectByte0:
		return src & 0x000000ff
	case insts.SDWASelectByte1:
		return (src & 0x0000ff00) >> 8
	case insts.SDWASelectByte2:
		return (src & 0x00ff0000) >> 16
	case insts.SDWASelectByte3:
		return (src & 0xff000000) >> 24
	case insts.SDWASelectWord0:
		return src & 0x0000ffff
	case insts.SDWASelectWord1:
		return (src & 0xffff0000) >> 16
	case insts.SDWASelectDWord:
		return src
	}
	return src
}

func (u *ALU) sdwaDstSelect(
	dstOld uint32,
	dstNew uint32,
	sel insts.SDWASelect,
	unused insts.SDWAUnused,
) uint32 {
	value := dstNew
	switch sel {
	case insts.SDWASelectByte0:
		value = value & 0x000000ff
	case insts.SDWASelectByte1:
		value = (value << 8) & 0x0000ff00
	case insts.SDWASelectByte2:
		value = (value << 16) & 0x00ff0000
	case insts.SDWASelectByte3:
		value = (value << 24) & 0xff000000
	case insts.SDWASelectWord0:
		value = value & 0x0000ffff
	case insts.SDWASelectWord1:
		value = (value << 16) & 0xffff0000
	}

	return value
}
