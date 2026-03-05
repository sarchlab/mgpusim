package emu

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// flatAddr computes the effective address for a flat/global instruction
// for a given lane. It handles:
//   - SAddr mode (scalar base + VGPR offset) vs OFF mode (VGPR pair as 64-bit addr)
//   - Signed 13-bit immediate offset (Offset0)
//
// GCN3 rules: SAddr is OFF when value is 0x7F or 0.
func (u *ALUImpl) flatAddr(state InstEmuState, laneID int) uint64 {
	inst := state.Inst()

	// Read the VGPR address component
	addr := state.ReadOperand(inst.Addr, laneID)

	// Check if SAddr is a valid scalar base register (not OFF)
	// GCN3: SAddr=0x7F or SAddr=0 means OFF mode
	if inst.SAddr != nil && inst.SAddr.IntValue != 0x7F && inst.SAddr.IntValue != 0 {
		// SAddr mode: addr = scalar_base + zero_extend(VGPR_32) + offset
		sAddrReg := int(inst.SAddr.IntValue)
		sAddrOperand := insts.NewSRegOperand(sAddrReg, sAddrReg, 2)
		scalarBase := state.ReadOperand(sAddrOperand, 0)
		// VGPR is 32-bit in SAddr mode, zero-extend it
		addr = scalarBase + (addr & 0xFFFFFFFF)
	}

	// Add signed immediate offset
	if inst.Offset0 != 0 {
		addr += uint64(int64(int32(inst.Offset0)))
	}

	return addr
}

//nolint:gocyclo
//nolint:funlen
func (u *ALUImpl) runFlat(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 16:
		u.runFlatLoadUByte(state)
	case 17:
		u.runFlatLoadSByte(state)
	case 18:
		u.runFlatLoadUShort(state)
	case 20:
		u.runFlatLoadDWord(state)
	case 21:
		u.runFlatLoadDWordX2(state)
	case 23:
		u.runFlatLoadDWordX4(state)
	case 28:
		u.runFlatStoreDWord(state)
	case 29:
		u.runFlatStoreDWordX2(state)
	case 30:
		u.runFlatStoreDWordX3(state)
	case 31:
		u.runFlatStoreDWordX4(state)
	default:
		log.Panicf("Opcode %d for FLAT format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runFlatLoadUByte(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		buf := u.storageAccessor.Read(pid, addr, 4)
		buf[1] = 0
		buf[2] = 0
		buf[3] = 0

		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALUImpl) runFlatLoadSByte(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		buf := u.storageAccessor.Read(pid, addr, 4)
		signedByte := int8(buf[0])
		extendedValue := int32(signedByte)
		result := insts.Uint32ToBytes(uint32(extendedValue))
		state.WriteOperandBytes(inst.Dst, i, result)
	}
}

func (u *ALUImpl) runFlatLoadUShort(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		buf := u.storageAccessor.Read(pid, addr, 4)
		buf[2] = 0
		buf[3] = 0

		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALUImpl) runFlatLoadDWord(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		buf := u.storageAccessor.Read(pid, addr, 4)
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALUImpl) runFlatLoadDWordX2(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		buf := u.storageAccessor.Read(pid, addr, 8)
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALUImpl) runFlatLoadDWordX4(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		buf := u.storageAccessor.Read(pid, addr, 16)
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALUImpl) runFlatStoreDWord(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		data := state.ReadOperandBytes(inst.Data, i, 4)
		u.storageAccessor.Write(pid, addr, data)
	}
}

func (u *ALUImpl) runFlatStoreDWordX2(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		data := state.ReadOperandBytes(inst.Data, i, 8)
		u.storageAccessor.Write(pid, addr, data)
	}
}

func (u *ALUImpl) runFlatStoreDWordX3(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		data := state.ReadOperandBytes(inst.Data, i, 12)
		u.storageAccessor.Write(pid, addr, data)
	}
}

func (u *ALUImpl) runFlatStoreDWordX4(state InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddr(state, i)
		data := state.ReadOperandBytes(inst.Data, i, 16)
		u.storageAccessor.Write(pid, addr, data)
	}
}
