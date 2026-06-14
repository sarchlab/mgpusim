package gcn3

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/bitops"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

//nolint:gocyclo,funlen
func (u *ALU) runSOP2(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSADDU32(state)
	case 1:
		u.runSSUBU32(state)
	case 2:
		u.runSADDI32(state)
	case 3:
		u.runSSUBI32(state)
	case 4:
		u.runSADDCU32(state)
	case 5:
		u.runSSUBBU32(state)
	case 6:
		u.runSMINI32(state)
	case 7:
		u.runSMINU32(state)
	case 8:
		u.runSMAXI32(state)
	case 9:
		u.runSMAXU32(state)
	case 10:
		u.runSCSELECTB32(state)
	case 12:
		u.runSANDB32(state)
	case 13:
		u.runSANDB64(state)
	case 15:
		u.runSORB64(state)
	case 16, 17:
		u.runSXORB64(state)
	case 19:
		u.runSANDN2B64(state)
	case 28:
		u.runSLSHLB32(state)
	case 29:
		u.runSLSHLB64(state)
	case 30:
		u.runSLSHRB32(state)
	case 31:
		u.runSLSHRB64(state)
	case 32:
		u.runSASHRI32(state)
	case 34:
		u.runSBFMB32(state)
	case 36:
		u.runSMULI32(state)
	case 38:
		u.runSBFEI32(state)
	default:
		log.Panicf("Opcode %d for SOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSADDU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))

	dst := src0 + src1
	var scc byte
	if src0 > math.MaxUint32-src1 {
		scc = 1
	} else {
		scc = 0
	}

	state.WriteOperand(inst.Dst, 0, uint64(dst))
	state.SetSCC(scc)
}

func (u *ALU) runSSUBU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	if src0 < src1 {
		state.SetSCC(1)
	}

	dst := src0 - src1
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runSADDI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))

	dst := src0 + src1
	var scc byte
	if src0 > math.MaxUint32-src1 {
		scc = 1
	} else {
		scc = 0
	}

	state.WriteOperand(inst.Dst, 0, uint64(dst))
	state.SetSCC(scc)
}

func (u *ALU) runSSUBI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := asInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := asInt32(uint32(state.ReadOperand(inst.Src1, 0)))
	dst := src0 - src1

	if src1 > 0 && dst > src0 {
		state.SetSCC(1)
	} else if src1 < 0 && dst < src0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}

	state.WriteOperand(inst.Dst, 0, uint64(int32ToBits(dst)))
}

func (u *ALU) runSADDCU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))
	scc := state.SCC()

	dst := src0 + src1 + uint32(scc)
	if src0 < math.MaxUint32-uint32(scc)-src1 {
		scc = 0
	} else {
		scc = 1
	}

	state.WriteOperand(inst.Dst, 0, uint64(dst))
	state.SetSCC(scc)
}

func (u *ALU) runSSUBBU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	scc := state.SCC()

	dst := src0 - src1 - uint64(scc)
	state.WriteOperand(inst.Dst, 0, dst)

	if src0 < src1+uint64(scc) {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSMINI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	src0i := asInt32(uint32(src0))
	src1i := asInt32(uint32(src1))

	if src0i < src1i {
		state.WriteOperand(inst.Dst, 0, src0)
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
	}
}

func (u *ALU) runSMINU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	if src0 < src1 {
		state.WriteOperand(inst.Dst, 0, src0)
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
	}
}

func (u *ALU) runSMAXI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	src0i := asInt32(uint32(src0))
	src1i := asInt32(uint32(src1))

	if src0i > src1i {
		state.WriteOperand(inst.Dst, 0, src0)
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
	}
}

func (u *ALU) runSMAXU32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	if src0 > src1 {
		state.WriteOperand(inst.Dst, 0, src0)
		state.SetSCC(1)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
	}
}

func (u *ALU) runSCSELECTB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)
	scc := state.SCC()

	if scc == 1 {
		state.WriteOperand(inst.Dst, 0, src0)
	} else {
		state.WriteOperand(inst.Dst, 0, src1)
	}
}

func (u *ALU) runSANDB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := src0 & src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSANDB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := src0 & src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSORB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := src0 | src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSXORB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := src0 ^ src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSANDN2B64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := src0 &^ src1
	state.WriteOperand(inst.Dst, 0, dst)
	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSLSHLB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := uint32(state.ReadOperand(inst.Src0, 0))
	src1 := uint8(state.ReadOperand(inst.Src1, 0))

	dst := uint64(src0 << (src1 & 0x1f))
	state.WriteOperand(inst.Dst, 0, dst)

	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSLSHLB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := uint8(state.ReadOperand(inst.Src1, 0))

	dst := src0 << (src1 & 0x3f)
	state.WriteOperand(inst.Dst, 0, dst)

	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSLSHRB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := src0 >> (src1 & 0x1f)
	state.WriteOperand(inst.Dst, 0, dst)

	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSLSHRB64(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := src0 >> (src1 & 0x3f)
	state.WriteOperand(inst.Dst, 0, dst)

	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSASHRI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := asInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := uint8(state.ReadOperand(inst.Src1, 0))

	dst := src0 >> src1
	state.WriteOperand(inst.Dst, 0, uint64(int32ToBits(dst)))

	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}

func (u *ALU) runSBFMB32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := ((uint64(1) << (src0 & 0x1f)) - 1) << (src1 & 0x1f)
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALU) runSMULI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := asInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := asInt32(uint32(state.ReadOperand(inst.Src1, 0)))

	dst := src0 * src1
	state.WriteOperand(inst.Dst, 0, uint64(int32ToBits(dst)))

	if src0 != 0 && dst/src0 != src1 {
		state.SetSCC(1)
	}
}

func (u *ALU) runSBFEI32(state emu.InstEmuState) {
	inst := state.Inst()
	src0 := asInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := uint32(state.ReadOperand(inst.Src1, 0))

	offset := bitops.ExtractBitsFromU32(src1, 0, 4)
	width := bitops.ExtractBitsFromU32(src1, 16, 22)
	dst := (src0 >> offset) & ((1 << width) - 1)

	state.WriteOperand(inst.Dst, 0, uint64(int32ToBits(dst)))

	if dst != 0 {
		state.SetSCC(1)
	} else {
		state.SetSCC(0)
	}
}
