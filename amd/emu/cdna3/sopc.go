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
	default:
		log.Panicf("Opcode %d for SOPC format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSCMPEQI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 == sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPLGI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 != sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPEQU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 == sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPLGU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 != sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPGTU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 > sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPLTU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 < sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPGEU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 >= sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPLEU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	if sp.SRC0 <= sp.SRC1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPLEI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	if src0 <= src1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPGEI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	if src0 >= src1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPLTI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	if src0 < src1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}

func (u *ALU) runSCMPGTI32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPC()
	src0 := emu.AsInt32(uint32(sp.SRC0))
	src1 := emu.AsInt32(uint32(sp.SRC1))
	if src0 > src1 {
		sp.SCC = 1
	} else {
		sp.SCC = 0
	}
}
