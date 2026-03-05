package cdna3

import (
	"encoding/binary"
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

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

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := state.ReadOperand(inst.Addr, i)
		buf := u.storageAccessor.Read(pid, addr, 1)

		result := make([]byte, 4)
		result[0] = buf[0]
		state.WriteOperandBytes(inst.Dst, i, result)
	}
}

func (u *ALU) runFlatLoadSByte(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := state.ReadOperand(inst.Addr, i)
		buf := u.storageAccessor.Read(pid, addr, 1)

		signedByte := int8(buf[0])
		result := make([]byte, 4)
		binary.LittleEndian.PutUint32(result, uint32(int32(signedByte)))
		state.WriteOperandBytes(inst.Dst, i, result)
	}
}

func (u *ALU) runFlatLoadUShort(state emu.InstEmuState) {
	inst := state.Inst()
	pid := state.PID()
	exec := state.EXEC()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := state.ReadOperand(inst.Addr, i)
		buf := u.storageAccessor.Read(pid, addr, 2)

		result := make([]byte, 4)
		result[0] = buf[0]
		result[1] = buf[1]
		state.WriteOperandBytes(inst.Dst, i, result)
	}
}

func (u *ALU) runFlatLoadDWord(state emu.InstEmuState) {
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

func (u *ALU) runFlatLoadDWordX2(state emu.InstEmuState) {
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

func (u *ALU) runFlatLoadDWordX4(state emu.InstEmuState) {
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

func (u *ALU) runFlatStoreDWord(state emu.InstEmuState) {
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

func (u *ALU) runFlatStoreDWordX2(state emu.InstEmuState) {
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

func (u *ALU) runFlatStoreDWordX3(state emu.InstEmuState) {
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

func (u *ALU) runFlatStoreDWordX4(state emu.InstEmuState) {
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
