package emu

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
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

		addr := state.ReadOperand(inst.Addr, i)
		data := state.ReadOperandBytes(inst.Data, i, 16)
		u.storageAccessor.Write(pid, addr, data)
	}
}
