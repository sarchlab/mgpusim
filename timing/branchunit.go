package timing

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/emu"
	"gitlab.com/akita/gcn3/timing/wavefront"
)

// A BranchUnit performs branch operations
type BranchUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
	alu                emu.ALU

	toRead  *wavefront.Wavefront
	toExec  *wavefront.Wavefront
	toWrite *wavefront.Wavefront

	isIdle bool
}

// NewBranchUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewBranchUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
	alu emu.ALU,
) *BranchUnit {
	u := new(BranchUnit)
	u.cu = cu
	u.scratchpadPreparer = scratchpadPreparer
	u.alu = alu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *BranchUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

func (u *BranchUnit) IsIdle() bool {
	u.isIdle = (u.toRead == nil) && (u.toWrite == nil) && (u.toExec == nil)
	return u.isIdle
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *BranchUnit) AcceptWave(
	wave *wavefront.Wavefront,
	now akita.VTimeInSec,
) {
	u.toRead = wave
	u.cu.logInstStageTask(now, wave.DynamicInst(), "issue", true)
	u.cu.logInstStageTask(now, wave.DynamicInst(), "read", false)
}

// Run executes three pipeline stages that are controlled by the BranchUnit
func (u *BranchUnit) Run(now akita.VTimeInSec) bool {
	madeProgress := false
	madeProgress = u.runWriteStage(now) || madeProgress
	madeProgress = u.runExecStage(now) || madeProgress
	madeProgress = u.runReadStage(now) || madeProgress
	return madeProgress
}

func (u *BranchUnit) runReadStage(now akita.VTimeInSec) bool {
	if u.toRead == nil {
		return false
	}

	if u.toExec == nil {
		u.scratchpadPreparer.Prepare(u.toRead, u.toRead)

		u.cu.logInstStageTask(now, u.toRead.DynamicInst(), "read", true)
		u.cu.logInstStageTask(now, u.toRead.DynamicInst(), "exec", false)

		u.toExec = u.toRead
		u.toRead = nil

		return true
	}
	return false
}

func (u *BranchUnit) runExecStage(now akita.VTimeInSec) bool {
	if u.toExec == nil {
		return false
	}

	if u.toWrite == nil {
		u.alu.Run(u.toExec)

		u.cu.logInstStageTask(now, u.toExec.DynamicInst(), "exec", true)
		u.cu.logInstStageTask(now, u.toExec.DynamicInst(), "write", false)

		u.toWrite = u.toExec
		u.toExec = nil
		return true
	}
	return false
}

func (u *BranchUnit) runWriteStage(now akita.VTimeInSec) bool {
	if u.toWrite == nil {
		return false
	}

	u.scratchpadPreparer.Commit(u.toWrite, u.toWrite)

	u.cu.logInstStageTask(now, u.toWrite.DynamicInst(), "write", true)
	u.cu.logInstTask(now, u.toRead, u.toWrite.DynamicInst(), true)

	u.toWrite.InstBuffer = nil
	u.cu.UpdatePCAndSetReady(u.toWrite)
	u.toWrite.InstBufferStartPC = u.toWrite.PC & 0xffffffffffffffc0
	u.toWrite = nil
	u.isIdle = false
	return true
}

func (u *BranchUnit) Flush() {
	u.toRead = nil
	u.toWrite = nil
	u.toExec = nil
}
