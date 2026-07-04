package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

//nolint:gocyclo
func (u *ALU) runSOPC(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSCMPEQI32(state)
	case 1:
		u.runSCMPLGI32(state)
	case 2:
		u.runSCMPGTI32(state)
	case 3:
		u.runSCMPGEI32(state)
	case 4:
		u.runSCMPLTI32(state)
	case 5:
		u.runSCMPLEI32(state)
	case 6:
		u.runSCMPEQU32(state)
	case 7:
		u.runSCMPLGU32(state)
	case 8:
		u.runSCMPGTU32(state)
	case 9:
		u.runSCMPGEU32(state)
	case 10:
		u.runSCMPLTU32(state)
	case 11:
		u.runSCMPLEU32(state)
	case 12:
		u.runSBITCMP0B32(state)
	case 13:
		u.runSBITCMP1B32(state)
	default:
		log.Panicf("Opcode %d for SOPC format is not implemented", inst.Opcode)
	}
}

// runSBITCMP0B32 implements s_bitcmp0_b32: SCC = (bit (Src1 & 31) of Src0 == 0).
func (u *ALU) runSBITCMP0B32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	bit := uint32(state.ReadOperand(inst.Src1, 0)) & 31
	if (src0>>bit)&1 == 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

// runSBITCMP1B32 implements s_bitcmp1_b32: SCC = (bit (Src1 & 31) of Src0 == 1).
func (u *ALU) runSBITCMP1B32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	bit := uint32(state.ReadOperand(inst.Src1, 0)) & 31
	if (src0>>bit)&1 == 1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPEQI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	if src0 == src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPLGI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	if src0 != src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPEQU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))
	if src0 == src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPLGU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))
	if src0 != src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPGTU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))
	if src0 > src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPLTU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))
	if src0 < src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPGEU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))
	if src0 >= src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPLEU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))
	if src0 <= src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPLEI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	if src0 <= src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPGEI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	if src0 >= src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPLTI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	if src0 < src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSCMPGTI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := emu.AsInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := emu.AsInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	if src0 > src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}
