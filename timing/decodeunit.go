package timing

import (
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/timing/wavefront"
)

// A DecodeUnit is any type of decode unit that takes one cycle to decode
type DecodeUnit struct {
	cu        *ComputeUnit
	ExecUnits []CUComponent // Execution units, index by SIMD number

	toDecode *wavefront.Wavefront
	decoded  bool

	isIdle bool
}

// NewDecodeUnit creates a new decode unit
func NewDecodeUnit(cu *ComputeUnit) *DecodeUnit {
	du := new(DecodeUnit)
	du.cu = cu
	du.decoded = false
	return du
}

// AddExecutionUnit registers an executions unit to the decode unit, so that
// the decode unit knows where to send the instruction to after decoding.
// This function has to be called in the order of SIMD number.
func (du *DecodeUnit) AddExecutionUnit(cuComponent CUComponent) {
	du.ExecUnits = append(du.ExecUnits, cuComponent)
}

// CanAcceptWave checks if the DecodeUnit is ready to decode another
// instruction
func (du *DecodeUnit) CanAcceptWave() bool {
	return du.toDecode == nil
}

func (du *DecodeUnit) IsIdle() bool {
	du.isIdle = (du.toDecode == nil) && (du.decoded == false)
	return du.isIdle
}

// AcceptWave takes a wavefront and decode the instruction in the next cycle
func (du *DecodeUnit) AcceptWave(
	wave *wavefront.Wavefront,
	now akita.VTimeInSec,
) {
	if du.toDecode != nil {
		log.Panicf("Decode unit busy, please run CanAcceptWave before accepting a wave")
	}

	du.toDecode = wave
	du.decoded = false
}

// Run decodes the instruction and sends the instruction to the next pipeline
// stage
func (du *DecodeUnit) Run(now akita.VTimeInSec) bool {
	if du.toDecode != nil {
		simdID := du.toDecode.SIMDID
		execUnit := du.ExecUnits[simdID]

		if execUnit.CanAcceptWave() {
			execUnit.AcceptWave(du.toDecode, now)
			du.toDecode = nil
			return true
		}
	}

	if du.toDecode != nil && !du.decoded {
		du.decoded = true
		return true
	}

	return false
}

func (du *DecodeUnit) Flush() {
	du.toDecode = nil
}
