package emu

import (
	"fmt"
	"log"
	"math"

	"gitlab.com/yaotsu/gcn3/insts"
)

// ALU is where the instructions get executed.
type ALU struct {
}

// Run executes the instruction in the scatchpad of the InstEmuState
func (u *ALU) Run(state InstEmuState) {
	inst := state.Inst()

	log.Println("before: ", u.dumpScratchpad(state, -1))

	switch inst.FormatType {
	case insts.Sop2:
		u.runSop2(state)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}

	log.Println("after : ", u.dumpScratchpad(state, -1))
}

func (u *ALU) runSop2(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runSADDU32(state)
	default:
		log.Panicf("Opcode %d for SOP2 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSADDU32(state InstEmuState) {
	sp := state.Scratchpad()

	src0 := insts.BytesToUint32(sp[0:8])
	src1 := insts.BytesToUint32(sp[8:16])

	dst := src0 + src1
	scc := byte(0)
	if src0 >= (1<<31) && src1 >= (1<<31) {
		scc = 1
	}

	copy(sp[16:24], insts.Uint32ToBytes(dst))
	sp[24] = scc
}

func (u *ALU) dumpScratchpad(state InstEmuState, byteCount int) string {
	scratchpad := state.Scratchpad()

	if byteCount <= 0 {
		byteCount = math.MaxInt32
	}

	i := 0
	output := ""
	for i < len(scratchpad) && i < byteCount {
		output += fmt.Sprintf("%02x ", scratchpad[i])
		i++
	}
	return output
}
