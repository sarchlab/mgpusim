package cdna3

import (
	"encoding/binary"
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

func (u *ALU) runDS(state emu.InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 0:
		u.runDSAddU32(state)
	case 13:
		u.runDSWRITEB32(state)
	case 14:
		u.runDSWRITE2B32(state)
	case 15:
		u.runDSWRITE2ST64B32(state)
	case 30:
		u.runDSWRITEB8(state)
	case 31:
		u.runDSWriteB16(state)
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
	case 223:
		u.runDSWRITEB128(state)
	case 254:
		u.runDSREADB96(state)
	case 255:
		u.runDSREADB128(state)
	default:
		log.Panicf("Opcode %d for DS format is not implemented", inst.Opcode)
	}
}

func (u *ALU) runDSWRITEB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		if addr0+4 > uint32(len(lds)) {
			continue // skip OOB lane
		}
		data := state.ReadOperandBytes(inst.Data, i, 4)
		copy(lds[addr0:addr0+4], data)
	}
}

func (u *ALU) runDSWRITE2B32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		baseAddr := uint32(state.ReadOperand(inst.Addr, i))
		addr0 := baseAddr + inst.Offset0*4
		addr1 := baseAddr + inst.Offset1*4

		if addr0+4 > uint32(len(lds)) || addr1+4 > uint32(len(lds)) {
			continue // skip OOB lane
		}

		data0 := state.ReadOperandBytes(inst.Data, i, 4)
		data1 := state.ReadOperandBytes(inst.Data1, i, 4)

		copy(lds[addr0:addr0+4], data0)
		copy(lds[addr1:addr1+4], data1)
	}
}

func (u *ALU) runDSWRITE2ST64B32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		baseAddr := uint32(state.ReadOperand(inst.Addr, i))
		// ST64: offset is multiplied by 64*4 = 256 bytes
		addr0 := baseAddr + inst.Offset0*256
		addr1 := baseAddr + inst.Offset1*256

		if addr0+4 > uint32(len(lds)) || addr1+4 > uint32(len(lds)) {
			continue // skip OOB lane
		}

		data0 := state.ReadOperandBytes(inst.Data, i, 4)
		data1 := state.ReadOperandBytes(inst.Data1, i, 4)

		copy(lds[addr0:addr0+4], data0)
		copy(lds[addr1:addr1+4], data1)
	}
}

func (u *ALU) runDSWRITEB8(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		if addr0+1 > uint32(len(lds)) {
			continue // skip OOB lane
		}
		data := state.ReadOperandBytes(inst.Data, i, 1)
		lds[addr0] = data[0]
	}
}

func (u *ALU) runDSREADB32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [4]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		if addr0+4 > uint32(len(lds)) {
			// Return zeros for OOB lane
			var zeros [4]byte
			state.WriteOperandBytes(inst.Dst, i, zeros[:])
			continue
		}
		copy(buf[:], lds[addr0:addr0+4])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}

func (u *ALU) runDSREAD2B32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [8]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		baseAddr := uint32(state.ReadOperand(inst.Addr, i))
		addr0 := baseAddr + inst.Offset0*4
		addr1 := baseAddr + inst.Offset1*4

		if addr0+4 > uint32(len(lds)) || addr1+4 > uint32(len(lds)) {
			var zeros [8]byte
			state.WriteOperandBytes(inst.Dst, i, zeros[:])
			continue
		}

		copy(buf[0:4], lds[addr0:addr0+4])
		copy(buf[4:8], lds[addr1:addr1+4])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}

func (u *ALU) runDSWRITE2B64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		baseAddr := uint32(state.ReadOperand(inst.Addr, i))
		addr0 := baseAddr + inst.Offset0*8
		addr1 := baseAddr + inst.Offset1*8

		if addr0+8 > uint32(len(lds)) || addr1+8 > uint32(len(lds)) {
			continue // skip OOB lane
		}

		data0 := state.ReadOperandBytes(inst.Data, i, 8)
		data1 := state.ReadOperandBytes(inst.Data1, i, 8)

		copy(lds[addr0:addr0+8], data0)
		copy(lds[addr1:addr1+8], data1)
	}
}

func (u *ALU) runDSREADB64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [8]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr := uint32(state.ReadOperand(inst.Addr, i))
		if addr+8 > uint32(len(lds)) {
			var zeros [8]byte
			state.WriteOperandBytes(inst.Dst, i, zeros[:])
			continue
		}
		copy(buf[:], lds[addr:addr+8])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}

func (u *ALU) runDSWRITEB128(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		if addr0+16 > uint32(len(lds)) {
			continue // skip OOB lane
		}
		data := state.ReadOperandBytes(inst.Data, i, 16)
		copy(lds[addr0:addr0+16], data)
	}
}

func (u *ALU) runDSREADB128(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [16]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		if addr0+16 > uint32(len(lds)) {
			var zeros [16]byte
			state.WriteOperandBytes(inst.Dst, i, zeros[:])
			continue
		}
		copy(buf[:], lds[addr0:addr0+16])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}

func (u *ALU) runDSREADB96(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [12]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		if addr0+12 > uint32(len(lds)) {
			var zeros [12]byte
			state.WriteOperandBytes(inst.Dst, i, zeros[:])
			continue
		}
		copy(buf[:], lds[addr0:addr0+12])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}

func (u *ALU) runDSAddU32(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		if addr0+4 > uint32(len(lds)) {
			continue // skip OOB lane
		}

		// Read old value
		oldVal := binary.LittleEndian.Uint32(lds[addr0 : addr0+4])

		// Read data to add
		dataBytes := state.ReadOperandBytes(inst.Data, i, 4)
		addVal := binary.LittleEndian.Uint32(dataBytes)

		// Write new value = old + add
		newVal := oldVal + addVal
		binary.LittleEndian.PutUint32(lds[addr0:addr0+4], newVal)

		// Return old value in Dst (if Dst is set)
		if inst.Dst != nil {
			var result [4]byte
			binary.LittleEndian.PutUint32(result[:], oldVal)
			state.WriteOperandBytes(inst.Dst, i, result[:])
		}
	}
}

func (u *ALU) runDSWriteB16(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		addr0 := uint32(state.ReadOperand(inst.Addr, i)) + inst.Offset0
		if addr0+2 > uint32(len(lds)) {
			continue // skip OOB lane
		}
		data := state.ReadOperandBytes(inst.Data, i, 2)
		copy(lds[addr0:addr0+2], data)
	}
}

func (u *ALU) runDSREAD2B64(state emu.InstEmuState) {
	inst := state.Inst()
	exec := state.EXEC()
	lds := u.LDS()

	var buf [16]byte
	for i := 0; i < 64; i++ {
		if exec&(1<<uint(i)) == 0 {
			continue
		}

		baseAddr := uint32(state.ReadOperand(inst.Addr, i))
		addr0 := baseAddr + inst.Offset0*8
		addr1 := baseAddr + inst.Offset1*8

		if addr0+8 > uint32(len(lds)) || addr1+8 > uint32(len(lds)) {
			var zeros [16]byte
			state.WriteOperandBytes(inst.Dst, i, zeros[:])
			continue
		}

		copy(buf[0:8], lds[addr0:addr0+8])
		copy(buf[8:16], lds[addr1:addr1+8])
		state.WriteOperandBytes(inst.Dst, i, buf[:])
	}
}
