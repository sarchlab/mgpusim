// Package cdna3 provides the CDNA3 (gfx942) ALU implementation.
package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// ALU is the CDNA3 (gfx942) ALU implementation.
type ALU struct {
	storageAccessor emu.StorageAccessor
	lds             []byte
}

// NewALU creates a new CDNA3 ALU instance.
func NewALU(storageAccessor emu.StorageAccessor) *ALU {
	return &ALU{storageAccessor: storageAccessor}
}

// ArchName returns the architecture name.
func (u *ALU) ArchName() string {
	return "CDNA3"
}

// SetLDS assigns the LDS storage to be used in the following instructions.
func (u *ALU) SetLDS(lds []byte) {
	u.lds = lds
}

// LDS returns the LDS storage.
func (u *ALU) LDS() []byte {
	return u.lds
}

// Run executes the instruction in the scratchpad of the InstEmuState.
//
//nolint:gocyclo
func (u *ALU) Run(state emu.InstEmuState) {
	inst := state.Inst()

	// Debug: log every instruction being executed (commented out for normal operation)
	// log.Printf("DEBUG ALU.Run: format=%s, opcode=%d, inst=%s", inst.FormatName, inst.Opcode, inst.InstName)

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
