package emu

import (
	"fmt"
	"log"
	"math"
)

func (u *ALUImpl) runVOP1(state InstEmuState) {
	inst := state.Inst()
	switch inst.Opcode {
	case 1:
		u.runVMOVB32(state)
	case 2:
		u.runVREADFIRSTLANEB32(state)
	case 6:
		u.runVCVTF32U32(state)
	case 7:
		u.runVCVTU32F32(state)
	case 8:
		u.runVCVTI32F32(state)
	case 28:
		u.runTRUNKF32(state)
	case 34, 35:
		u.runVRCPIFLAGF32(state)
	case 43:
		u.runVNOTB32(state)
	case 76:
		u.runVLOGLEGACYF32(state)
	default:
		log.Panicf("Opcode %d for VOP1 format is not implemented", inst.Opcode)
	}
}

func (u *ALUImpl) runVMOVB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = sp.SRC0[i]
	}
}

func (u *ALUImpl) runVREADFIRSTLANEB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	var laneid uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}
		laneid = i
		break
	}

	for i = 0; i < 64; i++ {
		sp.DST[i] = sp.SRC0[laneid]
	}
}

func (u *ALUImpl) runVCVTF32U32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		sp.DST[i] = uint64(math.Float32bits(float32(uint32(sp.SRC0[i]))))
	}
}

func (u *ALUImpl) runVCVTU32F32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		var dst uint64
		if math.IsNaN(float64(src)) {
			dst = 0
		} else if src < 0 {
			dst = 0
		} else if uint64(src) > math.MaxUint32 {
			dst = math.MaxUint32
		} else {
			dst = uint64(src)
		}

		sp.DST[i] = dst
	}
}

func (u *ALUImpl) runVCVTI32F32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		var dst uint64
		if math.IsNaN(float64(src)) || math.IsNaN(float64(0-src)) {
			dst = 0
		} else if int32(src) > math.MaxInt32 {
			dst = math.MaxInt32
		} else if int32(src) < (0 - math.MaxInt32) {
			dst = uint64(int32ToBits(0 - math.MaxInt32))
		} else {
			dst = uint64(int32ToBits(int32(src)))
		}

		sp.DST[i] = dst
	}
}

func (u *ALUImpl) runTRUNKF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()
	for i := uint(0); i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := float32(math.Trunc(float64(src)))
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runVRCPIFLAGF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := 1 / src
		sp.DST[i] = uint64(math.Float32bits(dst))
	}
}

func (u *ALUImpl) runVNOTB32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	for i = 0; i < 64; i++ {
		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := uint32(sp.SRC0[i])
		dst := ^src
		sp.DST[i] = uint64(uint32(dst))
	}
}

func (u *ALUImpl) runVLOGLEGACYF32(state InstEmuState) {
	sp := state.Scratchpad().AsVOP1()

	var i uint
	fmt.Printf("\n")
	for i = 0; i < 64; i++ {
		fmt.Printf("0x%x, ", sp.SRC0[i])

		if !laneMasked(sp.EXEC, i) {
			continue
		}

		src := math.Float32frombits(uint32(sp.SRC0[i]))
		dst := math.Log2(float64(src))
		sp.DST[i] = uint64(math.Float32bits(float32(dst)))
	}
	fmt.Printf("\n")
}
