package cdna3

import (
	"log"
	"math/bits"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func (u *ALU) runDS(state emu.InstEmuState) {
	inst := state.Inst()
	access := inst.LDSAccess()
	lds := u.LDS()
	src := [2]*insts.Operand{inst.Data, inst.Data1}
	var result [16]byte
	for active := state.EXEC(); active != 0; active &= active - 1 {
		lane := bits.TrailingZeros64(active)
		base := uint32(state.ReadOperand(inst.Addr, lane))
		for n, off := range access.Offsets[:access.Count] {
			begin := int(base + off)
			end := begin + access.Bytes
			if end > len(lds) {
				log.Panicf("%s: LDS address %#x + %d exceeds LDS size %d (lane %d)",
					inst.InstName, begin, access.Bytes, len(lds), lane)
			}
			if access.Write {
				copy(lds[begin:end], state.ReadOperandBytes(src[n], lane, access.Bytes))
			} else {
				copy(result[n*access.Bytes:], lds[begin:end])
			}
		}
		if !access.Write {
			state.WriteOperandBytes(inst.Dst, lane, result[:access.Count*access.Bytes])
		}
	}
}
