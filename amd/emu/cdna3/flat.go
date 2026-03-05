package cdna3

import (
	"encoding/binary"
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// flatPrecomputeScalarBase reads the scalar base register once (lane-invariant).
// Returns (hasSAddr, scalarBase). If hasSAddr is false, scalarBase is unused.
// CDNA3 rules: SAddr is OFF only when value is 0x7F.
func (u *ALU) flatPrecomputeScalarBase(
	state emu.InstEmuState,
) (bool, uint64) {
	inst := state.Inst()
	if inst.SAddr != nil && inst.SAddr.IntValue != 0x7F {
		sAddrReg := int(inst.SAddr.IntValue)
		sAddrOperand := insts.NewSRegOperand(sAddrReg, sAddrReg, 2)
		scalarBase := state.ReadOperand(sAddrOperand, 0)
		return true, scalarBase
	}
	return false, 0
}

// flatAddr computes the effective address for a flat/global instruction
// for a given lane. It handles:
//   - SAddr mode (scalar base + VGPR offset) vs OFF mode (VGPR pair as 64-bit addr)
//   - Signed 13-bit immediate offset (Offset0)
//
// CDNA3 rules: SAddr is OFF only when value is 0x7F.
func (u *ALU) flatAddr(state emu.InstEmuState, laneID int) uint64 {
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)
	return u.flatAddrWithScalar(state, laneID, hasSAddr, scalarBase)
}

// flatAddrWithScalar computes the effective address using a precomputed
// scalar base, avoiding redundant scalar register reads across lanes.
func (u *ALU) flatAddrWithScalar(
	state emu.InstEmuState, laneID int,
	hasSAddr bool, scalarBase uint64,
) uint64 {
	inst := state.Inst()

	// Read the VGPR address component
	addr := state.ReadOperand(inst.Addr, laneID)

	if hasSAddr {
		// SAddr mode: addr = scalar_base + zero_extend(VGPR_32) + offset
		addr = scalarBase + (addr & 0xFFFFFFFF)
	}

	// Add signed immediate offset
	if inst.Offset0 != 0 {
		addr += uint64(int64(int32(inst.Offset0)))
	}

	return addr
}

//nolint:gocyclo
func (u *ALU) runFlat(state emu.InstEmuState) {
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

func (u *ALU) runFlatLoadUByte(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	var result [4]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		buf := u.storageAccessor.Read(pid, addr, 1)

		result[0] = buf[0]
		result[1] = 0
		result[2] = 0
		result[3] = 0
		state.WriteOperandBytes(inst.Dst, i, result[:])
	}
}

func (u *ALU) runFlatLoadSByte(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	var result [4]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		buf := u.storageAccessor.Read(pid, addr, 1)

		signedByte := int8(buf[0])
		binary.LittleEndian.PutUint32(result[:], uint32(int32(signedByte)))
		state.WriteOperandBytes(inst.Dst, i, result[:])
	}
}

func (u *ALU) runFlatLoadUShort(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	var result [4]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		buf := u.storageAccessor.Read(pid, addr, 2)

		result[0] = buf[0]
		result[1] = buf[1]
		result[2] = 0
		result[3] = 0
		state.WriteOperandBytes(inst.Dst, i, result[:])
	}
}

func (u *ALU) runFlatLoadDWord(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		buf := u.storageAccessor.Read(pid, addr, 4)
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALU) runFlatLoadDWordX2(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		buf := u.storageAccessor.Read(pid, addr, 8)
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALU) runFlatLoadDWordX4(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		buf := u.storageAccessor.Read(pid, addr, 16)
		state.WriteOperandBytes(inst.Dst, i, buf)
	}
}

func (u *ALU) runFlatStoreDWord(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 4)
		u.storageAccessor.Write(pid, addr, data)
	}
}

func (u *ALU) runFlatStoreDWordX2(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 8)
		u.storageAccessor.Write(pid, addr, data)
	}
}

func (u *ALU) runFlatStoreDWordX3(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 12)
		u.storageAccessor.Write(pid, addr, data)
	}
}

func (u *ALU) runFlatStoreDWordX4(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()
	hasSAddr, scalarBase := u.flatPrecomputeScalarBase(state)

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := u.flatAddrWithScalar(state, i, hasSAddr, scalarBase)
		data := state.ReadOperandBytes(inst.Data, i, 16)
		u.storageAccessor.Write(pid, addr, data)
	}
}
