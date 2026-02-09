package cdna3

import (
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
)

func (u *ALU) runDS(state emu.InstEmuState) {
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

func (u *ALU) runDSWRITEB32(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()
	lds := u.LDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + inst.Offset0
		data0offset := uint(8 + 64*4)
		copy(lds[addr0:addr0+4], sp[data0offset+i*16:data0offset+i*16+4])
	}
}

func (u *ALU) runDSWRITE2B32(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()
	lds := u.LDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + inst.Offset0*4
		data0offset := uint(8 + 64*4)
		addr1 := layout.ADDR[i] + inst.Offset1*4
		data1offset := uint(8 + 64*4 + 256*4)

		copy(lds[addr0:addr0+4], sp[data0offset+i*16:data0offset+i*16+4])
		copy(lds[addr1:addr1+4], sp[data1offset+i*16:data1offset+i*16+4])
	}
}

func (u *ALU) runDSWRITEB8(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()
	lds := u.LDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + inst.Offset0
		dstOffset := uint(8 + 64*4)
		data := sp[dstOffset+i*16]
		lds[addr0] = data
	}
}

func (u *ALU) runDSREADB32(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()
	lds := u.LDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + inst.Offset0
		dstOffset := uint(8 + 64*4 + 256*4*2)
		copy(sp[dstOffset+i*16:dstOffset+i*16+4], lds[addr0:addr0+4])
	}
}

func (u *ALU) runDSREAD2B32(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()
	lds := u.LDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + inst.Offset0*4
		dstOffset := uint(8 + 64*4 + 256*4*2)
		copy(sp[dstOffset+i*16:dstOffset+i*16+4], lds[addr0:addr0+4])

		addr1 := layout.ADDR[i] + inst.Offset1*4
		copy(sp[dstOffset+i*16+4:dstOffset+i*16+8], lds[addr1:addr1+4])
	}
}

func (u *ALU) runDSWRITE2B64(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()
	lds := u.LDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + inst.Offset0*8
		data0Offset := uint(8 + 64*4)
		copy(lds[addr0:addr0+8], sp[data0Offset+i*16:data0Offset+i*16+8])

		addr1 := layout.ADDR[i] + inst.Offset1*8
		data1Offset := uint(8 + 64*4 + 256*4)
		copy(lds[addr1:addr1+8], sp[data1Offset+i*16:data1Offset+i*16+8])
	}
}

func (u *ALU) runDSREADB64(state emu.InstEmuState) {
	sp := state.Scratchpad()
	layout := sp.AsDS()
	lds := u.LDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(layout.EXEC, i) {
			continue
		}

		addr := layout.ADDR[i]
		dstOffset := uint(8 + 64*4 + 256*4*2)
		copy(sp[dstOffset+i*16:dstOffset+i*16+8], lds[addr:addr+8])
	}
}

func (u *ALU) runDSREAD2B64(state emu.InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()
	lds := u.LDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !emu.LaneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + inst.Offset0*8
		dstOffset := uint(8 + 64*4 + 256*4*2)
		copy(sp[dstOffset+i*16:dstOffset+i*16+8], lds[addr0:addr0+8])

		addr1 := layout.ADDR[i] + inst.Offset1*8
		copy(sp[dstOffset+i*16+8:dstOffset+i*16+16], lds[addr1:addr1+8])
	}
}
