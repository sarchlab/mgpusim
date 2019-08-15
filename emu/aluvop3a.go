package emu

import (
	"log"
	"math"
	"strings"
)

func (u *ALUImpl) runVOP3A(state InstEmuState) {
	inst := state.Inst()

	u.vop3aPreprocess(state)

	switch inst.Opcode {
	case 65: // 0x41
		u.runVCmpLtF32VOP3a(state)
	case 193: // 0xC1
		u.runVCmpLtI32VOP3a(state)
	case 196: // 0xC4
		u.runVCmpGtI32VOP3a(state)
	case 201: // 0xC9
		u.runVCmpLtU32VOP3a(state)
	case 202: // 0xCA
		u.runVCmpEqU32VOP3a(state)
	case 203: // 0xCB
		u.runVCmpLeU32VOP3a(state)
	case 204: // 0xCC
		u.runVCmpGtU32VOP3a(state)
	case 205: // 0xCD
		u.runVCmpLgU32VOP3a(state)
	case 206: // 0xCE
		u.runVCmpGeU32VOP3a(state)
	case 233: // 0xE9
		u.runVCmpLtU64VOP3a(state)
	case 256:
		u.runVCNDMASKB32VOP3a(state)
	case 451, 488:
		u.runVMADU64U32(state)
	case 460:
		u.runVFMAF64(state)
	case 479:
		u.runVDIVFIXUPF64(state)
	case 483:
		u.runVDIVFMASF64(state)
	case 640:
		u.runVADDF64(state)
	case 641:
		u.runVMULF64(state)
	case 645:
		u.runVMULLOU32(state)
	case 646:
		u.runVMULHIU32(state)
	case 655:
		u.runVLSHLREVB64(state)
	case 657:
		u.runVASHRREVI64(state)
	default:
		log.Panicf("Opcode %d for VOP3a format is not implemented", inst.Opcode)
	}
	u.vop3aPostprocess(state)
}

func (u *ALUImpl) vop3aPreprocess(state InstEmuState) {
	inst := state.Inst()
	sp := state.Scratchpad().AsVOP3A()

	if inst.Abs != 0 {
		if strings.Contains(inst.InstName, "F32") {
			if inst.Abs&0x1 != 0 {
				for i := 0; i < 64; i++ {
					src0 := math.Float32frombits(uint32(sp.SRC0[i]))
					src0 = float32(math.Abs(float64(src0)))
					sp.SRC0[i] = uint64(math.Float32bits(src0))
				}
			}

			if inst.Abs&0x2 != 0 {
				for i := 0; i < 64; i++ {
					src1 := math.Float32frombits(uint32(sp.SRC1[i]))
					src1 = float32(math.Abs(float64(src1)))
					sp.SRC1[i] = uint64(math.Float32bits(src1))
				}
			}

			if inst.Abs&0x4 != 0 {
				for i := 0; i < 64; i++ {
					src2 := math.Float32frombits(uint32(sp.SRC2[i]))
					src2 = float32(math.Abs(float64(src2)))
					sp.SRC2[i] = uint64(math.Float32bits(src2))
				}
			}
		} else {
			log.Printf("Absolute operation for %s is not implemented.", inst.InstName)
		}
	}

	if inst.Neg != 0 {
		if strings.Contains(inst.InstName, "F64") || strings.Contains(inst.InstName, "f64") {
			if inst.Neg&0x1 != 0 {
				for i := 0; i < 64; i++ {

					//src0 := math.Float64frombits(uint64(sp.SRC0[i]) & 0x8000000000000000)
					src0 := math.Float64frombits(uint64(sp.SRC0[i]))
					src0 = src0 * (-1.0)
					sp.SRC0[i] = uint64(math.Float64bits(src0))
				}
			}

			if inst.Neg&0x2 != 0 {
				for i := 0; i < 64; i++ {
					src1 := math.Float64frombits(uint64(sp.SRC1[i]))
					src1 = src1 * (-1.0)
					sp.SRC1[i] = uint64(math.Float64bits(src1))
				}
			}

			if inst.Neg&0x4 != 0 {
				for i := 0; i < 64; i++ {
					src2 := math.Float64frombits(uint64(sp.SRC2[i]))
					src2 = src2 * (-1.0)
					sp.SRC2[i] = uint64(math.Float64bits(src2))
				}
			}
		} else {
			log.Printf("Negative operation for %s is not implemented.", inst.InstName)
		}

	}
}

func (u *ALUImpl) vop3aPostprocess(state InstEmuState) {
	inst := state.Inst()

	if inst.Omod != 0 {
		log.Panic("Output modifiers are not supported.")
	}
}

func (u *ALUImpl) runVCmpLtF32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	sp.VCC = 0
	var i uint
	var src0, src1 float32
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}
		src0 = math.Float32frombits(uint32(sp.SRC0[i]))
		src1 = math.Float32frombits(uint32(sp.SRC1[i]))
		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLtI32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := asInt32(uint32(sp.SRC0[i]))
		src1 := asInt32(uint32(sp.SRC1[i]))

		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpGtI32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := asInt32(uint32(sp.SRC0[i]))
		src1 := asInt32(uint32(sp.SRC1[i]))

		if src0 > src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLtU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpEqU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if uint32(src0) == uint32(src1) {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLeU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 <= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpGtU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 > src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLgU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 != src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpGeU32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 >= src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCmpLtU64VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src0 := sp.SRC0[i]
		src1 := sp.SRC1[i]

		if src0 < src1 {
			sp.DST[0] |= (1 << i)
		}
	}
}

func (u *ALUImpl) runVCNDMASKB32VOP3a(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		if (sp.SRC2[i] & (1 << i)) > 0 {
			sp.DST[i] = sp.SRC1[i]
		} else {
			sp.DST[i] = sp.SRC0[i]
		}

	}
}

func (u *ALUImpl) runVMADU64U32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC0[i]*sp.SRC1[i] + sp.SRC2[i]
	}
}

func (u *ALUImpl) runVMULLOU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = (sp.SRC0[i] * sp.SRC1[i])
	}
}

func (u *ALUImpl) runVMULHIU32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = (sp.SRC0[i] * sp.SRC1[i]) >> 32
	}

}

func (u *ALUImpl) runVLSHLREVB64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC1[i] << sp.SRC0[i]
	}
}

func (u *ALUImpl) runVASHRREVI64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = int64ToBits(asInt64(sp.SRC1[i]) >> sp.SRC0[i])
	}
}

func (u *ALUImpl) runVADDF64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !laneMasked(sp.EXEC, i) {
				continue
			}

			src0 := math.Float64frombits(uint64(sp.SRC0[i]))
			src1 := math.Float64frombits(uint64(sp.SRC1[i]))
			dst := src0 + src1
			sp.DST[i] = uint64(math.Float64bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVFMAF64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := math.Float64frombits(uint64(sp.SRC0[i]))
			src1 := math.Float64frombits(uint64(sp.SRC1[i]))
			src2 := math.Float64frombits(uint64(sp.SRC2[i]))

			dst := src0*src1 + src2
			sp.DST[i] = uint64(math.Float64bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVMULF64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()
	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !laneMasked(sp.EXEC, i) {
				continue
			}
			src0 := math.Float64frombits(uint64(sp.SRC0[i]))
			src1 := math.Float64frombits(uint64(sp.SRC1[i]))

			dst := src0 * src1
			sp.DST[i] = uint64(math.Float64bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)

	}
}

func (u *ALUImpl) runVDIVFMASF64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()

	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !laneMasked(sp.EXEC, i) {
				continue
			}

			vccVal := (sp.VCC) & (1 << i)

			src0 := math.Float64frombits(uint64(sp.SRC0[i]))
			src1 := math.Float64frombits(uint64(sp.SRC1[i]))
			src2 := math.Float64frombits(uint64(sp.SRC2[i]))

			var dst float64
			if vccVal == 1 {
				dst = (float64)(math.Pow(2.0, 64)) * (src0*src1 + src2)
			} else {
				dst = src0*src1 + src2
			}
			sp.DST[i] = uint64(math.Float64bits(dst))
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}

func (u *ALUImpl) runVDIVFIXUPF64(state InstEmuState) {
	sp := state.Scratchpad().AsVOP3A()
	inst := state.Inst()

	if inst.IsSdwa == false {
		var i uint
		for i = 0; i < 64; i++ {
			if !laneMasked(sp.EXEC, i) {
				continue
			}

			signS1 := uint64(sp.SRC1[i]) >> 63
			signS2 := uint64(sp.SRC2[i]) >> 63
			signOut := (signS1) ^ (signS2)

			src0 := math.Float64frombits(uint64(sp.SRC0[i]))
			src1 := math.Float64frombits(uint64(sp.SRC1[i]))
			src2 := math.Float64frombits(uint64(sp.SRC2[i]))

			exponentSrc1 := uint64((uint64(sp.SRC1[i]) << 1) >> 53)
			exponentSrc2 := uint64((uint64(sp.SRC2[i]) << 1) >> 53)

			var dst float64

			// Double Precision => Nan = 0x7FFFFFFFFFFFFFFF

			if src2 == 0x7FFFFFFFFFFFFFFF {
				dst = 0x7FF8000000000001 // assign a NaN value with quieting
			} else if src1 == 0x7FFFFFFFFFFFFFFF {
				dst = 0x7FF8000000000001
			} else if (src1 == 0) && (src2 == 0) {
				// 0 / 0
				dst = 0xFFF8000000000000 // undetermined value
			} else if (math.Abs(src1) == 0x7FF0000000000000 || math.Abs(src1) == 0xFFF0000000000000) &&
				(math.Abs(src2) == 0x7FF0000000000000 || math.Abs(src2) == 0xFFF0000000000000) {
				// inf / inf
				dst = 0xFFF8000000000000
			} else if src1 == 0 || (math.Abs(src2) == 0x7FF0000000000000 || math.Abs(src2) == 0xFFF0000000000000) {
				// x/0 , or inf / y
				if signOut == 1 {
					dst = 0xFFF0000000000000 // -INF
				} else {
					dst = 0x7FF0000000000000 // +INF
				}
			} else if (math.Abs(src1) == 0x7FF0000000000000 || math.Abs(src1) == 0xFFF0000000000000) || (src2 == 0) {
				// x/inf, 0/y
				if signOut == 1 {
					dst = 0x8000000000000000 // -0
				} else {
					dst = 0x0000000000000000 // +0
				}
			} else if int64(exponentSrc2-exponentSrc1) < -1075 {
				log.Panicf("Underflow for VOP3A instruction opcode %d not implemented \n", inst.Opcode)
				if signOut == 1 {
					//-underflow
				} else {
					//+underflow
				}
			} else if exponentSrc1 == 2047 {
				log.Panicf("Overflow for VOP3A instruction opcode %d not implemented \n", inst.Opcode)
				if signOut == 1 {
					//-overflow
				} else {
					//+overflow
				}
			} else {
				if signOut == 1 {
					dst = math.Abs(src0) * (-1.0)
				} else {
					dst = math.Abs(src0)
				}
			}

			sp.DST[i] = math.Float64bits(dst)
		}
	} else {
		log.Panicf("SDWA for VOP3A instruction opcode  %d not implemented \n", inst.Opcode)
	}
}
