package emu

import (
	"bytes"
	"fmt"
	"log"

	"encoding/binary"

	"math"

	"gitlab.com/yaotsu/gcn3/insts"
)

// ALU is where the instructions get executed.
type ALU struct {
}

// Run executes the instruction in the scatchpad of the InstEmuState
func (u *ALU) Run(state InstEmuState) {
	inst := state.Inst()

	switch inst.FormatType {
	case insts.Sop2:
		u.runSop2(state)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}

}

func (u *ALU) runSop2(state InstEmuState) {
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
