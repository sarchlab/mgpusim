package emu

import "log"

func (u *ALUImpl) runDS(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 78:
		u.runDSWRITE2B64(state)
	case 119:
		u.runDSREAD2B64(state)
	default:
		log.Panicf("Opcode %d for DS format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runDSWRITE2B64(state InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !u.laneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + uint32(inst.Offset0)*8
		data0Offset := uint(8 + 64*4)
		copy(u.LDS[addr0:addr0+8], sp[data0Offset+i*16:data0Offset+i*16+8])

		addr1 := layout.ADDR[i] + uint32(inst.Offset1)*8
		data1Offset := uint(8 + 64*4 + 256*4)
		copy(u.LDS[addr1:addr1+8], sp[data1Offset+i*16:data1Offset+i*16+8])
	}
}

func (u *ALUImpl) runDSREAD2B64(state InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad()
	layout := sp.AsDS()

	i := uint(0)
	for i = 0; i < 64; i++ {
		if !u.laneMasked(layout.EXEC, i) {
			continue
		}

		addr0 := layout.ADDR[i] + uint32(inst.Offset0)*8
		dstOffset := uint(8 + 64*4 + 256*4*2)
		copy(sp[dstOffset+i*16:dstOffset+i*16+8], u.LDS[addr0:addr0+8])

		addr1 := layout.ADDR[i] + uint32(inst.Offset1)*8
		copy(sp[dstOffset+i*16+8:dstOffset+i*16+16], u.LDS[addr1:addr1+8])
	}
}
