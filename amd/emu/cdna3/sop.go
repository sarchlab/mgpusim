package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

func (u *ALU) runSMEM(state emu.InstEmuState) {
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
	case 4:
		u.runSLOADDWORDX16(state)
	default:
		log.Panicf("Opcode %d for SMEM format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runSLOADDWORD(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	pid := state.PID()
	buf := u.storageAccessor.Read(pid, sp.Base+sp.Offset, 4)
	sp.DST[0] = insts.BytesToUint32(buf)
}

func (u *ALU) runSLOADDWORDX2(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()
	pid := state.PID()
	addr := sp.Base + sp.Offset
	buf := u.storageAccessor.Read(pid, addr, 8)
	// log.Printf("DEBUG s_load_dwordx2: pid=%d, base=0x%016x, offset=0x%x, addr=0x%016x", pid, sp.Base, sp.Offset, addr)
	// log.Printf("DEBUG   loaded bytes: %v", buf)
	copy(spRaw[32:40], buf)
}

func (u *ALU) runSLOADDWORDX4(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()
	pid := state.PID()
	buf := u.storageAccessor.Read(pid, sp.Base+sp.Offset, 16)
	copy(spRaw[32:48], buf)
}

func (u *ALU) runSLOADDWORDX8(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()
	pid := state.PID()
	buf := u.storageAccessor.Read(pid, sp.Base+sp.Offset, 32)
	copy(spRaw[32:64], buf)
}

func (u *ALU) runSLOADDWORDX16(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSMEM()
	spRaw := state.Scratchpad()
	pid := state.PID()
	buf := u.storageAccessor.Read(pid, sp.Base+sp.Offset, 64)
	copy(spRaw[32:96], buf)
}

//nolint:gocyclo
func (u *ALU) runSOPP(state emu.InstEmuState) {
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

func (u *ALU) runSCBRANCH(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := emu.AsInt16(uint16(sp.IMM & 0xffff))
	sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
}

func (u *ALU) runSCBRANCHSCC0(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := emu.AsInt16(uint16(sp.IMM & 0xffff))
	if sp.SCC == 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALU) runSCBRANCHSCC1(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := emu.AsInt16(uint16(sp.IMM & 0xffff))
	if sp.SCC == 1 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALU) runSCBRANCHVCCZ(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := emu.AsInt16(uint16(sp.IMM & 0xffff))
	if sp.VCC == 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALU) runSCBRANCHVCCNZ(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := emu.AsInt16(uint16(sp.IMM & 0xffff))
	if sp.VCC != 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALU) runSCBRANCHEXECZ(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := emu.AsInt16(uint16(sp.IMM & 0xffff))
	if sp.EXEC == 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}

func (u *ALU) runSCBRANCHEXECNZ(state emu.InstEmuState) {
	sp := state.Scratchpad().AsSOPP()
	imm := emu.AsInt16(uint16(sp.IMM & 0xffff))
	if sp.EXEC != 0 {
		sp.PC = uint64(int64(sp.PC) + int64(imm)*4)
	}
}
