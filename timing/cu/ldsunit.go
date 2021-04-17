package cu

import (
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mgpusim/v2/emu"
	"gitlab.com/akita/mgpusim/v2/timing/wavefront"
)

// A LDSUnit performs Scalar operations
type LDSUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
	alu                emu.ALU

	toRead  *wavefront.Wavefront
	toExec  *wavefront.Wavefront
	toWrite *wavefront.Wavefront

	isIdle bool
}

// NewLDSUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewLDSUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
	alu emu.ALU,
) *LDSUnit {
	u := new(LDSUnit)
	u.cu = cu
	u.scratchpadPreparer = scratchpadPreparer
	u.alu = alu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *LDSUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// IsIdle checks idleness
func (u *LDSUnit) IsIdle() bool {
	u.isIdle = (u.toRead == nil) && (u.toWrite == nil) && (u.toExec == nil)
	return u.isIdle
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *LDSUnit) AcceptWave(wave *wavefront.Wavefront, now sim.VTimeInSec) {
	u.toRead = wave
}

// Run executes three pipeline stages that are controlled by the LDSUnit
func (u *LDSUnit) Run(now sim.VTimeInSec) bool {
	madeProgress := false
	madeProgress = u.runWriteStage(now) || madeProgress
	madeProgress = u.runExecStage(now) || madeProgress
	madeProgress = u.runReadStage(now) || madeProgress
	return madeProgress
}

func (u *LDSUnit) runReadStage(now sim.VTimeInSec) bool {
	if u.toRead == nil {
		return false
	}

	if u.toExec == nil {
		u.scratchpadPreparer.Prepare(u.toRead, u.toRead)

		u.toExec = u.toRead
		u.toRead = nil
		return true
	}
	return false
}

func (u *LDSUnit) runExecStage(now sim.VTimeInSec) bool {
	if u.toExec == nil {
		return false
	}

	if u.toWrite == nil {
		u.alu.SetLDS(u.toExec.WG.LDS)
		u.alu.Run(u.toExec)

		u.toWrite = u.toExec
		u.toExec = nil
		return true
	}
	return false
}

func (u *LDSUnit) runWriteStage(now sim.VTimeInSec) bool {
	if u.toWrite == nil {
		return false
	}

	u.scratchpadPreparer.Commit(u.toWrite, u.toWrite)

	u.cu.logInstTask(now, u.toWrite, u.toWrite.DynamicInst(), true)

	u.cu.UpdatePCAndSetReady(u.toWrite)

	u.toWrite = nil
	return true
}

// Flush clears the unit
func (u *LDSUnit) Flush() {
	u.toRead = nil
	u.toExec = nil
	u.toWrite = nil
}
