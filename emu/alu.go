package emu

import (
	"bytes"
	"fmt"
	"log"

	"encoding/binary"

	"math"

	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

// ALU is where the instructions get executed.
type ALU struct {
	Storage *mem.Storage
}

// Run executes the instruction in the scatchpad of the InstEmuState
func (u *ALU) Run(state InstEmuState) {
	inst := state.Inst()

	switch inst.FormatType {
	case insts.Sop2:
		u.runSOP2(state)
	case insts.Smem:
		u.runSMEM(state)
	case insts.Vop1:
		u.runVOP1(state)
	case insts.Vop2:
		u.runVOP2(state)
	case insts.Vop3:
		u.runVOP3A(state)
	case insts.Flat:
		u.runFlat(state)
	case insts.Sopp:
		u.runSOPP(state)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}

}

func (u *ALU) runSOP2(state InstEmuState) {
	log.Println("before: ", u.dumpScratchpadAsSop2(state, -1))
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSADDU32(state)
	case 4:
		u.runSADDCU32(state)
	default:
		log.Panicf("Opcode %d for SOP2 format is not implemented", inst.Opcode)
	}
	log.Println("after : ", u.dumpScratchpadAsSop2(state, -1))
}

func (u *ALU) runSADDU32(state InstEmuState) {
	sp := state.Scratchpad()

	src0 := insts.BytesToUint32(sp[0:8])
	src1 := insts.BytesToUint32(sp[8:16])

	dst := src0 + src1
	scc := byte(0)
	if src0 > math.MaxUint32-src1 {
		scc = 1
	} else {
		scc = 0
	}

	copy(sp[16:24], insts.Uint32ToBytes(dst))
	sp[24] = scc
}

func (u *ALU) runSADDCU32(state InstEmuState) {
	sp := state.Scratchpad()

	src0 := insts.BytesToUint32(sp[0:8])
	src1 := insts.BytesToUint32(sp[8:16])
	scc := sp[24]

	dst := src0 + src1 + uint32(scc)
	if src0 < math.MaxUint32-uint32(scc)-src1 {
		scc = 0
	} else {
		scc = 1
	}

	copy(sp[16:24], insts.Uint32ToBytes(dst))
	sp[24] = scc
}

func (u *ALU) runVOP1(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 1:
		u.runVMOVB32(state)
	default:
		log.Panicf("Opcode %d for VOP1 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runVMOVB32(state InstEmuState) {
	sp := state.Scratchpad()
	copy(sp[512:1024], sp[0:512])
}

func (u *ALU) runVOP2(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 25:
		u.runVADDI32(state)
	case 28:
		u.runVADDCU32(state)
	default:
		log.Panicf("Opcode %d for VOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runVADDI32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()

	for i := 0; i < 64; i++ {
		src0 := asInt32(uint32(sp.SRC0[i]))
		src1 := asInt32(uint32(sp.SRC1[i]))

		if (src1 > 0 && src0 > math.MaxInt32-src1) ||
			(src1 < 0 && src0 < math.MinInt32+src1) {
			sp.VCC |= 1 << uint32(i)
		}

		sp.DST[i] = uint64(int32ToBits(src0 + src1))
	}
}

func (u *ALU) runVADDCU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP2()

	for i := 0; i < 64; i++ {
		carry := (sp.VCC & (1 << uint(i))) >> uint(i)

		if sp.SRC0[i] > math.MaxUint32-carry-sp.SRC1[i] {
			sp.VCC |= 1 << uint32(i)
		}

		sp.DST[i] = sp.SRC0[i] + sp.SRC1[i] + carry
	}
}

func (u *ALU) runVOP3A(state InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 645:
		u.runVMULLOU32(state)
	case 657:
		u.runVASHRREVI64(state)
	default:
		log.Panicf("Opcode %d for VOP3a format is not implemented", inst.Opcode)
	}

	u.vop3aPostprocess(state)
}

func (u *ALU) vop3aPreprocess(state InstEmuState) {
}

func (u *ALU) vop3aPostprocess(state InstEmuState) {
}

func (u *ALU) runVMULLOU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	for i := 0; i < 64; i++ {
		sp.DST[i] = sp.SRC0[i] * sp.SRC1[i]
	}
}

func (u *ALU) runVASHRREVI64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	for i := 0; i < 64; i++ {
		sp.DST[i] = int64ToBits(asInt64(sp.SRC1[i]) >> sp.SRC0[i])
	}
}

func (u *ALU) runFlat(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 18:
		u.runFlatLoadUShort(state)
	case 20:
		u.runFlatLoadDWord(state)
	default:
		log.Panicf("Opcode %d for FLAT format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runFlatLoadUShort(state InstEmuState) {
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

func (u *ALU) runFlatLoadDWord(state InstEmuState) {
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

func (u *ALU) runSMEM(state InstEmuState) {
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

func (u *ALU) runSLOADDWORD(state InstEmuState) {
	sp := state.Scratchpad().AsSMEM()

	buf, err := u.Storage.Read(sp.Base+sp.Offset, 4)
	if err != nil {
		log.Panic(err)
	}

	sp.DST[0] = insts.BytesToUint32(buf)
}

func (u *ALU) runSLOADDWORDX2(state InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()

	buf, err := u.Storage.Read(sp.Base+sp.Offset, 8)
	if err != nil {
		log.Panic(err)
	}

	copy(spRaw[32:40], buf)
}

func (u *ALU) runSOPP(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0: // S_NOP
	// Do nothing
	case 12: // S_WAITCNT
	// Do nothing
	default:
		log.Panicf("Opcode %d for SOPP format is not implemented", inst.Opcode)
	}

}

func (u *ALU) dumpScratchpadAsSop2(state InstEmuState, byteCount int) string {
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
