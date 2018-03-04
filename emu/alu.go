package emu

import (
	"bytes"
	"fmt"
	"log"

	"encoding/binary"

	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

type ALU interface {
	Run(state InstEmuState)
}

// ALU is where the instructions get executed.
type ALUImpl struct {
	Storage *mem.Storage
}

// NewALU creates a new ALU with a storage as a dependency.
func NewALUImpl(storage *mem.Storage) *ALUImpl {
	alu := new(ALUImpl)
	alu.Storage = storage
	return alu
}

// Run executes the instruction in the scatchpad of the InstEmuState
func (u *ALUImpl) Run(state InstEmuState) {
	inst := state.Inst()

	switch inst.FormatType {
	case insts.Sop1:
		u.runSOP1(state)
	case insts.Sop2:
		u.runSOP2(state)
	case insts.Sopc:
		u.runSOPC(state)
	case insts.Smem:
		u.runSMEM(state)
	case insts.Vop1:
		u.runVOP1(state)
	case insts.Vop2:
		u.runVOP2(state)
	case insts.Vop3:
		u.runVOP3A(state)
	case insts.Vopc:
		u.runVOPC(state)
	case insts.Flat:
		u.runFlat(state)
	case insts.Sopp:
		u.runSOPP(state)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}

}

func (u *ALUImpl) runFlat(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 18:
		u.runFlatLoadUShort(state)
	case 20:
		u.runFlatLoadDWord(state)
	case 28:
		u.runFlatStoreDWord(state)
	default:
		log.Panicf("Opcode %d for FLAT format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runFlatLoadUShort(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	for i := 0; i < 64; i++ {
		buf, err := u.Storage.Read(sp.ADDR[i], uint64(4))
		if err != nil {
			log.Panic(err)
		}

		buf[2] = 0
		buf[3] = 0

		sp.DST[i*4] = insts.BytesToUint32(buf)
	}
}

func (u *ALUImpl) runFlatLoadDWord(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	for i := 0; i < 64; i++ {
		buf, err := u.Storage.Read(sp.ADDR[i], uint64(4))
		if err != nil {
			log.Panic(err)
		}

		sp.DST[i*4] = insts.BytesToUint32(buf)
	}
}

func (u *ALUImpl) runFlatStoreDWord(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	for i := 0; i < 64; i++ {
		err := u.Storage.Write(sp.ADDR[i], insts.Uint32ToBytes(sp.DATA[i*4]))
		if err != nil {
			log.Panic(err)
		}
	}
}

func (u *ALUImpl) runSMEM(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSLOADDWORD(state)
	case 1:
		u.runSLOADDWORDX2(state)
	default:
		log.Panicf("Opcode %d for SMEM format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runSLOADDWORD(state InstEmuState) {
	sp := state.Scratchpad().AsSMEM()

	buf, err := u.Storage.Read(sp.Base+sp.Offset, 4)
	if err != nil {
		log.Panic(err)
	}

	sp.DST[0] = insts.BytesToUint32(buf)
}

func (u *ALUImpl) runSLOADDWORDX2(state InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()

	buf, err := u.Storage.Read(sp.Base+sp.Offset, 8)
	if err != nil {
		log.Panic(err)
	}

	copy(spRaw[32:40], buf)
}

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

func (u *ALUImpl) laneMasked(Exec uint64, laneID uint) bool {
	return Exec&(1<<laneID) > 0
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
