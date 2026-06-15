package emu

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v4/amd/bitops"
)

//nolint:gocyclo,funlen
func (u *ALUImpl) runSOP2(state InstEmuState) {
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

func (u *ALUImpl) runSADDU32(state InstEmuState) {
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

func (u *ALUImpl) runSSUBU32(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	if src0 < src1 {
		state.SetSCC(1)
	}

	dst := src0 - src1
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runSADDI32(state InstEmuState) {
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

func (u *ALUImpl) runSSUBI32(state InstEmuState) {
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

func (u *ALUImpl) runSADDCU32(state InstEmuState) {
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

func (u *ALUImpl) runSSUBBU32(state InstEmuState) {
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

func (u *ALUImpl) runSMINI32(state InstEmuState) {
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

func (u *ALUImpl) runSMINU32(state InstEmuState) {
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

func (u *ALUImpl) runSMAXI32(state InstEmuState) {
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

func (u *ALUImpl) runSMAXU32(state InstEmuState) {
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

func (u *ALUImpl) runSCSELECTB32(state InstEmuState) {
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

func (u *ALUImpl) runSANDB32(state InstEmuState) {
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

func (u *ALUImpl) runSANDB64(state InstEmuState) {
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

func (u *ALUImpl) runSORB64(state InstEmuState) {
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

func (u *ALUImpl) runSXORB64(state InstEmuState) {
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

func (u *ALUImpl) runSANDN2B64(state InstEmuState) {
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

func (u *ALUImpl) runSLSHLB32(state InstEmuState) {
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

func (u *ALUImpl) runSLSHLB64(state InstEmuState) {
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

func (u *ALUImpl) runSLSHRB32(state InstEmuState) {
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

func (u *ALUImpl) runSLSHRB64(state InstEmuState) {
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

func (u *ALUImpl) runSASHRI32(state InstEmuState) {
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

func (u *ALUImpl) runSBFMB32(state InstEmuState) {
	inst := state.Inst()
	src0 := state.ReadOperand(inst.Src0, 0)
	src1 := state.ReadOperand(inst.Src1, 0)

	dst := ((uint64(1) << (src0 & 0x1f)) - 1) << (src1 & 0x1f)
	state.WriteOperand(inst.Dst, 0, dst)
}

func (u *ALUImpl) runSMULI32(state InstEmuState) {
	inst := state.Inst()
	src0 := asInt32(uint32(state.ReadOperand(inst.Src0, 0)))
	src1 := asInt32(uint32(state.ReadOperand(inst.Src1, 0)))

	dst := src0 * src1
	state.WriteOperand(inst.Dst, 0, uint64(int32ToBits(dst)))

	if src0 != 0 && dst/src0 != src1 {
		state.SetSCC(1)
	}
}

func (u *ALUImpl) runSBFEI32(state InstEmuState) {
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
