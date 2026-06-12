package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
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
	case 12: // s_bitcmp0_b32
		u.runSBITCMP0B32(state)
	case 13: // s_bitcmp1_b32
		u.runSBITCMP1B32(state)
	case 16: // s_cmp_eq_u64 (CDNA3 opcode)
		u.runSCMPEQU64(state)
	case 17: // s_cmp_lg_u64 (CDNA3 opcode)
		u.runSCMPNEU64(state)
	case 18: // s_cmp_eq_u64 (legacy opcode)
		u.runSCMPEQU64(state)
	case 19: // s_cmp_ne_u64 (legacy opcode)
		u.runSCMPNEU64(state)
	default:
		log.Panicf("Opcode %d for SOPC format is not implemented", inst.Opcode)
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

// runSCMPEQU64 implements s_cmp_eq_u64 (opcode 18)
// SCC = (S0.u64 == S1.u64)
func (u *ALU) runSCMPEQU64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	if src0 == src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

// runSCMPNEU64 implements s_cmp_ne_u64 (opcode 19)
// SCC = (S0.u64 != S1.u64)
func (u *ALU) runSCMPNEU64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	if src0 != src1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

// runSBITCMP0B32 implements s_bitcmp0_b32 (opcode 12)
// SCC = (S0.u32[S1.u32[4:0]] == 0)
func (u *ALU) runSBITCMP0B32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0)) & 0x1F
	if (src0>>src1)&1 == 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

// runSBITCMP1B32 implements s_bitcmp1_b32 (opcode 13)
// SCC = (S0.u32[S1.u32[4:0]] == 1)
func (u *ALU) runSBITCMP1B32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0)) & 0x1F
	if (src0>>src1)&1 == 1 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}
