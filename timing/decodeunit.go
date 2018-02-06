package timing

import (
	"log"

	"gitlab.com/yaotsu/core"
)

// A DecodeUnit is any type of decode unit that takes one cycle to decode
type DecodeUnit struct {
	ExecUnits []CUComponent // Execution units, index by SIMD number

	toDecode *Wavefront
}

// NewDecodeUnit creates a new decode unit
func NewDecodeUnit() *DecodeUnit {
	du := new(DecodeUnit)
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

// AcceptWave takes a wavefront and decode the instruction in the next cycle
func (du *DecodeUnit) AcceptWave(wave *Wavefront) {
	if du.toDecode != nil {
		log.Panicf("Decode unit busy, please run CanAcceptWave before accepting a wave")
	}
	du.toDecode = wave
}

// Run decodes the instruction and sends the instruction to the next pipeline
// stage
func (du *DecodeUnit) Run(now core.VTimeInSec) {
	simdID := du.toDecode.SIMDID
	execUnit := du.ExecUnits[simdID]

	if execUnit.CanAcceptWave() {
		execUnit.AcceptWave(du.toDecode)
		du.toDecode = nil
	}
}
