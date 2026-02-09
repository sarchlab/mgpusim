package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

//nolint:gocyclo
func (u *ALU) runSOP1(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 3:
		u.runSMOVB32(state)
	case 4:
		u.runSMOVB64(state)
	case 7:
		u.runSNOTU32(state)
	case 12:
		u.runSBREVB32(state)
	case 28:
		u.runSGETPCB64(state)
	case 32, 33:
		u.runSANDSAVEEXECB64(state)
	case 34:
		u.runSORSAVEEXECB64(state)
	case 35:
		u.runSXORSAVEEXECB64(state)
	case 36:
		u.runSANDN2SAVEEXECB64(state)
	case 37:
		u.runSORN2SAVEEXECB64(state)
	case 38:
		u.runSNANDSAVEEXECB64(state)
	case 39:
		u.runSNORSAVEEXECB64(state)
	case 40:
		u.runSNXORSAVEEXECB64(state)
	default:
		log.Panicf("Opcode %d for SOP1 format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSMOVB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.SRC0
}

func (u *ALU) runSMOVB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.SRC0
}

func (u *ALU) runSNOTU32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = ^sp.SRC0
	if sp.DST != 0 {
		sp.SCC = 1
	}
}

func (u *ALU) runSBREVB32(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	src := uint32(sp.SRC0)
	var dst uint32
	for i := 0; i < 32; i++ {
		if (src & (1 << i)) != 0 {
			dst |= 1 << (31 - i)
		}
	}
	sp.DST = uint64(dst)
}

func (u *ALU) runSGETPCB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.PC
}

func (u *ALU) runSANDSAVEEXECB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = sp.SRC0 & sp.EXEC
	if sp.EXEC == 0 {
		sp.SCC = 0
	} else {
		sp.SCC = 1
	}
}

func (u *ALU) runSORSAVEEXECB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = sp.SRC0 | sp.EXEC
	if sp.EXEC == 0 {
		sp.SCC = 0
	} else {
		sp.SCC = 1
	}
}

func (u *ALU) runSXORSAVEEXECB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = sp.SRC0 ^ sp.EXEC
	if sp.EXEC == 0 {
		sp.SCC = 0
	} else {
		sp.SCC = 1
	}
}

func (u *ALU) runSANDN2SAVEEXECB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = sp.SRC0 & ^sp.EXEC
	if sp.EXEC == 0 {
		sp.SCC = 0
	} else {
		sp.SCC = 1
	}
}

func (u *ALU) runSORN2SAVEEXECB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = sp.SRC0 | ^sp.EXEC
	if sp.EXEC == 0 {
		sp.SCC = 0
	} else {
		sp.SCC = 1
	}
}

func (u *ALU) runSNANDSAVEEXECB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = ^(sp.SRC0 & sp.EXEC)
	if sp.EXEC == 0 {
		sp.SCC = 0
	} else {
		sp.SCC = 1
	}
}

func (u *ALU) runSNORSAVEEXECB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = ^(sp.SRC0 | sp.EXEC)
	if sp.EXEC == 0 {
		sp.SCC = 0
	} else {
		sp.SCC = 1
	}
}

func (u *ALU) runSNXORSAVEEXECB64(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOP1()
	sp.DST = sp.EXEC
	sp.EXEC = ^(sp.SRC0 ^ sp.EXEC)
	if sp.EXEC == 0 {
		sp.SCC = 0
	} else {
		sp.SCC = 1
	}
}
