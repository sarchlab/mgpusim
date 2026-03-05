package emu

import (
	"log"
)

func (u *ALUImpl) runDS(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 13:
		u.runDSWRITEB32(state)
	case 14:
		u.runDSWRITE2B32(state)
	case 30:
		u.runDSWRITEB8(state)
	case 54:
		u.runDSREADB32(state)
	case 55:
		u.runDSREAD2B32(state)
	case 78:
		u.runDSWRITE2B64(state)
	case 118:
		u.runDSREADB64(state)
	case 119:
		u.runDSREAD2B64(state)
	default:
		log.Panicf("Opcode %d for DS format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runDSWRITEB32(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		data := state.ReadOperandBytes(inst.Data, i, 4)
		copy(lds[addr0:addr0+4], data)
	}
}

func (u *ALUImpl) runDSWRITE2B32(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0*4
		data0 := state.ReadOperandBytes(inst.Data, i, 4)
		copy(lds[addr0:addr0+4], data0)

		addr1 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset1*4
		data1 := state.ReadOperandBytes(inst.Data1, i, 4)
		copy(lds[addr1:addr1+4], data1)
	}
}

func (u *ALUImpl) runDSWRITEB8(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		data := state.ReadOperandBytes(inst.Data, i, 1)
		lds[addr0] = data[0]
	}
}

func (u *ALUImpl) runDSREADB32(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [4]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		copy(buf[:], lds[addr0:addr0+4])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}

func (u *ALUImpl) runDSREAD2B32(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [8]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0*4
		addr1 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset1*4

		copy(buf[0:4], lds[addr0:addr0+4])
		copy(buf[4:8], lds[addr1:addr1+4])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}

func (u *ALUImpl) runDSWRITE2B64(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0*8
		data0 := state.ReadOperandBytes(inst.Data, i, 8)
		copy(lds[addr0:addr0+8], data0)

		addr1 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset1*8
		data1 := state.ReadOperandBytes(inst.Data1, i, 8)
		copy(lds[addr1:addr1+8], data1)
	}
}

func (u *ALUImpl) runDSREADB64(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [8]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := uint32(state.ReadOperand(inst.Addr, i))
		copy(buf[:], lds[addr:addr+8])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}

func (u *ALUImpl) runDSREAD2B64(state InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [16]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0*8
		addr1 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset1*8

		copy(buf[0:8], lds[addr0:addr0+8])
		copy(buf[8:16], lds[addr1:addr1+8])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}
