package emu

import (
	"bytes"
	"fmt"
	"log"

	"encoding/binary"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// ALU does its jobs
type ALU interface {
	Run(state InstEmuState)

	SetLDS(lds []byte)
	LDS() []byte
}

// ALUImpl is where the instructions get executed.
type ALUImpl struct {
	storageAccessor *StorageAccessor
	lds             []byte
}

// NewALU creates a new ALU with a storage as a dependency.
func NewALU(storageAccessor *StorageAccessor) *ALUImpl {
	alu := new(ALUImpl)
	alu.storageAccessor = storageAccessor
	return alu
}

// SetLDS assigns the LDS storage to be used in the following instructions.
func (u *ALUImpl) SetLDS(lds []byte) {
	u.lds = lds
}

// LDS returns lds
func (u *ALUImpl) LDS() []byte {
	return u.lds
}

// Run executes the instruction in the scatchpad of the InstEmuState
//
//nolint:gocyclo
func (u *ALUImpl) Run(state InstEmuState) {
	inst := state.Inst()
	// fmt.Printf("%s\n", inst.String(nil))

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

func (u *ALUImpl) runSMEM(state InstEmuState) {
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
	default:
		log.Panicf("Opcode %d for SMEM format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runSLOADDWORD(state InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, sp.Base+sp.Offset, 4)

	sp.DST[0] = insts.BytesToUint32(buf)
}

func (u *ALUImpl) runSLOADDWORDX2(state InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, sp.Base+sp.Offset, 8)
	copy(spRaw[32:40], buf)
}

func (u *ALUImpl) runSLOADDWORDX4(state InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, sp.Base+sp.Offset, 16)
	copy(spRaw[32:48], buf)
}

func (u *ALUImpl) runSLOADDWORDX8(state InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()
	pid := state.PID()

	buf := u.storageAccessor.Read(pid, sp.Base+sp.Offset, 32)
	copy(spRaw[32:64], buf)
}

//nolint:gocyclo
func (u *ALUImpl) runSOPP(state InstEmuState) {
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

func (u *ALUImpl) runSCBRANCH(state InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
}

func (u *ALUImpl) runSCBRANCHSCC0(state InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	if sp.SCC == 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALUImpl) runSCBRANCHSCC1(state InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	if sp.SCC == 1 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALUImpl) runSCBRANCHVCCZ(state InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	if sp.VCC == 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALUImpl) runSCBRANCHVCCNZ(state InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	if sp.VCC != 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALUImpl) runSCBRANCHEXECZ(state InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	if sp.EXEC == 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALUImpl) runSCBRANCHEXECNZ(state InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := asInt16(uint16(sp.IMM & 0xffff))
	if sp.EXEC != 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func laneMasked(Exec uint64, laneID uint) bool {
	return Exec&(1<<laneID) > 0
}

func (u *ALUImpl) sdwaSrcSelect(src uint32, sel insts.SDWASelect) uint32 {
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

func (u *ALUImpl) sdwaDstSelect(
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

func (u *ALUImpl) dumpScratchpadAsSop2(state InstEmuState, byteCount int) string {
	scratchpad := state.Scratchpad()
	layout := new(SOP2Layout)

	binary.Read(bytes.NewBuffer(scratchpad), binary.LittleEndian, layout)

	output := fmt.Sprintf(
		`
			SRC0: 0x%[1]x(%[1]d),
			SRC1: 0x%[2]x(%[2]d),
			SCC: 0x%[3]x(%[3]d),
			DST: 0x%[4]x(%[4]d)\n",
		`,
		layout.SRC0, layout.SRC1, layout.SCC, layout.DST)

	return output
}
