package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

func (u *ALU) runSOPK(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSMOVKI32(state)
	case 1:
		u.runSCMOVKI32(state)
	case 2:
		u.runSCMPKEQI32(state)
	case 3:
		u.runSCMPKLGI32(state)
	case 4:
		u.runSCMPKGTI32(state)
	case 5:
		u.runSCMPKGEI32(state)
	case 6:
		u.runSCMPKLTI32(state)
	case 7:
		u.runSCMPKLEI32(state)
	case 8:
		u.runSCMPKEQU32(state)
	case 9:
		u.runSCMPKLGU32(state)
	case 10:
		u.runSCMPKGTU32(state)
	case 11:
		u.runSCMPKGEU32(state)
	case 12:
		u.runSCMPKLTU32(state)
	case 13:
		u.runSCMPKLEU32(state)
	case 14:
		u.runSADDKI32(state)
	case 15:
		u.runSMULKI32(state)
	default:
		log.Panicf("Opcode %d for SOPK format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSMOVKI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int16ToBits(imm)))
}

func (u *ALU) runSCMOVKI32(state emu.InstEmuState) {
	inst := state.Inst()
	if state.SCC() == 1 {
		imm := emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff))
		state.WriteOperand(inst.Dst, 0, uint64(emu.Int16ToBits(imm)))
	}
}

func (u *ALU) runSCMPKEQI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := int32(emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff)))
	src := emu.AsInt32(uint32(state.ReadOperand(inst.Dst, 0)))
	if src == imm {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPKLGI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := int32(emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff)))
	src := emu.AsInt32(uint32(state.ReadOperand(inst.Dst, 0)))
	if src != imm {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

// setSCCBool sets SCC to 1 when cond is true, else 0.
func setSCCBool(state emu.InstEmuState, cond bool) {
	if cond {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

// sopkImmI32 decodes the SOPK 16-bit immediate, sign-extended to int32.
func sopkImmI32(state emu.InstEmuState) int32 {
	return int32(emu.AsInt16(uint16(state.ReadOperand(state.Inst().SImm16, 0) & 0xffff)))
}

// s_cmpk_{gt,ge,lt,le}_i32: signed compare of Dst (read as i32) against the
// sign-extended SImm16, writing the result to SCC.
func (u *ALU) runSCMPKGTI32(state emu.InstEmuState) {
	src := emu.AsInt32(uint32(state.ReadOperand(state.Inst().Dst, 0)))
	setSCCBool(state, src > sopkImmI32(state))
}

func (u *ALU) runSCMPKGEI32(state emu.InstEmuState) {
	src := emu.AsInt32(uint32(state.ReadOperand(state.Inst().Dst, 0)))
	setSCCBool(state, src >= sopkImmI32(state))
}

func (u *ALU) runSCMPKLTI32(state emu.InstEmuState) {
	src := emu.AsInt32(uint32(state.ReadOperand(state.Inst().Dst, 0)))
	setSCCBool(state, src < sopkImmI32(state))
}

func (u *ALU) runSCMPKLEI32(state emu.InstEmuState) {
	src := emu.AsInt32(uint32(state.ReadOperand(state.Inst().Dst, 0)))
	setSCCBool(state, src <= sopkImmI32(state))
}

// s_cmpk_{eq,lg,gt,ge,lt,le}_u32: unsigned compare of Dst (read as u32) against
// the SImm16 sign-extended then reinterpreted as u32.
func (u *ALU) runSCMPKEQU32(state emu.InstEmuState) {
	src := uint32(state.ReadOperand(state.Inst().Dst, 0))
	setSCCBool(state, src == uint32(sopkImmI32(state)))
}

func (u *ALU) runSCMPKLGU32(state emu.InstEmuState) {
	src := uint32(state.ReadOperand(state.Inst().Dst, 0))
	setSCCBool(state, src != uint32(sopkImmI32(state)))
}

func (u *ALU) runSCMPKGTU32(state emu.InstEmuState) {
	src := uint32(state.ReadOperand(state.Inst().Dst, 0))
	setSCCBool(state, src > uint32(sopkImmI32(state)))
}

func (u *ALU) runSCMPKGEU32(state emu.InstEmuState) {
	src := uint32(state.ReadOperand(state.Inst().Dst, 0))
	setSCCBool(state, src >= uint32(sopkImmI32(state)))
}

func (u *ALU) runSCMPKLTU32(state emu.InstEmuState) {
	src := uint32(state.ReadOperand(state.Inst().Dst, 0))
	setSCCBool(state, src < uint32(sopkImmI32(state)))
}

func (u *ALU) runSCMPKLEU32(state emu.InstEmuState) {
	src := uint32(state.ReadOperand(state.Inst().Dst, 0))
	setSCCBool(state, src <= uint32(sopkImmI32(state)))
}

// runSADDKI32 implements s_addk_i32: Dst = Dst + signext(SImm16), with SCC set
// on signed overflow.
func (u *ALU) runSADDKI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := int32(emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff)))
	src := emu.AsInt32(uint32(state.ReadOperand(inst.Dst, 0)))
	result := src + imm
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(result)))

	if (src > 0 && imm > 0 && result < 0) ||
		(src < 0 && imm < 0 && result >= 0) {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSMULKI32(state emu.InstEmuState) {
	inst := state.Inst()
	imm := int32(emu.AsInt16(uint16(state.ReadOperand(inst.SImm16, 0) & 0xffff)))
	src := emu.AsInt32(uint32(state.ReadOperand(inst.Dst, 0)))
	result := src * imm
	state.WriteOperand(inst.Dst, 0, uint64(emu.Int32ToBits(result)))
}
