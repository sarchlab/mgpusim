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

// sdwaSrcSelect applies SDWA source selection to extract the appropriate sub-dword.
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

// sdwaDstSelect applies SDWA destination selection to place the result in the
// appropriate sub-dword position, handling unused bits according to dst_unused.
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
	case insts.SDWASelectDWord:
		return dstNew
	default:
		return dstNew
	}

	// For now, always use PAD semantics (zero unused bits)
	// The reference implementation doesn't seem to handle PRESERVE or SEXT either
	return value
}
