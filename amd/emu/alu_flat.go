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
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		buf := u.storageAccessor.Read(pid, sp.ADDR[i], uint64(4))
		buf[1] = 0
		buf[2] = 0
		buf[3] = 0

		sp.DST[i*4] = insts.BytesToUint32(buf)
	}
}

func (u *ALUImpl) runFlatLoadSByte(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}
		buf := u.storageAccessor.Read(pid, sp.ADDR[i], uint64(4))
		signedByte := int8(buf[0])
		extendedValue := int32(signedByte)
		sp.DST[i*4] = uint32(extendedValue)
	}
}

func (u *ALUImpl) runFlatLoadUShort(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		buf := u.storageAccessor.Read(pid, sp.ADDR[i], uint64(4))

		buf[2] = 0
		buf[3] = 0

		sp.DST[i*4] = insts.BytesToUint32(buf)
	}
}

func (u *ALUImpl) runFlatLoadDWord(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		buf := u.storageAccessor.Read(pid, sp.ADDR[i], uint64(4))
		sp.DST[i*4] = insts.BytesToUint32(buf)
	}
}

func (u *ALUImpl) runFlatLoadDWordX2(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		buf := u.storageAccessor.Read(pid, sp.ADDR[i], uint64(8))

		sp.DST[i*4] = insts.BytesToUint32(buf[0:4])
		sp.DST[i*4+1] = insts.BytesToUint32(buf[4:8])
	}
}

func (u *ALUImpl) runFlatLoadDWordX4(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		buf := u.storageAccessor.Read(pid, sp.ADDR[i], uint64(16))

		sp.DST[i*4] = insts.BytesToUint32(buf[0:4])
		sp.DST[i*4+1] = insts.BytesToUint32(buf[4:8])
		sp.DST[i*4+2] = insts.BytesToUint32(buf[8:12])
		sp.DST[i*4+3] = insts.BytesToUint32(buf[12:16])
	}
}

func (u *ALUImpl) runFlatStoreDWord(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()

	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		u.storageAccessor.Write(
			pid, sp.ADDR[i], insts.Uint32ToBytes(sp.DATA[i*4]))
	}
}

func (u *ALUImpl) runFlatStoreDWordX2(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		buf := make([]byte, 8)
		copy(buf[0:4], insts.Uint32ToBytes(sp.DATA[i*4]))
		copy(buf[4:8], insts.Uint32ToBytes(sp.DATA[(i*4)+1]))

		u.storageAccessor.Write(pid, sp.ADDR[i], buf)
	}
}

func (u *ALUImpl) runFlatStoreDWordX3(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		buf := make([]byte, 12)
		copy(buf[0:4], insts.Uint32ToBytes(sp.DATA[i*4]))
		copy(buf[4:8], insts.Uint32ToBytes(sp.DATA[(i*4)+1]))
		copy(buf[8:12], insts.Uint32ToBytes(sp.DATA[(i*4)+2]))

		u.storageAccessor.Write(pid, sp.ADDR[i], buf)
	}
}

func (u *ALUImpl) runFlatStoreDWordX4(state InstEmuState) {
	sp := state.Scratchpad().AsFlat()
	pid := state.PID()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		buf := make([]byte, 16)
		copy(buf[0:4], insts.Uint32ToBytes(sp.DATA[i*4]))
		copy(buf[4:8], insts.Uint32ToBytes(sp.DATA[(i*4)+1]))
		copy(buf[8:12], insts.Uint32ToBytes(sp.DATA[(i*4)+2]))
		copy(buf[12:16], insts.Uint32ToBytes(sp.DATA[(i*4)+3]))

		u.storageAccessor.Write(pid, sp.ADDR[i], buf)
	}
}
