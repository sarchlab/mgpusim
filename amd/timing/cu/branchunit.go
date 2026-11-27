package cu

import (
	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

// branchPipelineCycles is the number of cycles from issuing a branch to its
// write stage.
const branchPipelineCycles = 3

// A BranchUnit performs branch operations
type BranchUnit struct {
	cu *ComputeUnit

	alu emu.ALU

	// takenIssueGap is the number of cycles between issuing a taken branch
	// and the earliest issue of its target. Zero keeps the legacy behavior.
	takenIssueGap int
}

// NewBranchUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewBranchUnit(
	cu *ComputeUnit,
	alu emu.ALU,
) *BranchUnit {
	u := new(BranchUnit)
	u.cu = cu
	u.alu = alu
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *BranchUnit) CanAcceptWave() bool {
	return u.state().Read == 0
}

// IsIdle checks idleness
func (u *BranchUnit) IsIdle() bool {
	u.state().Idle = (u.state().Read == 0) && (u.state().Write == 0) && (u.state().Exec == 0)
	return u.state().Idle
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *BranchUnit) AcceptWave(
	wave *Wavefront,
) {
	u.state().Read = u.cu.useWave(wave)
}

// Run executes three pipeline stages that are controlled by the BranchUnit
func (u *BranchUnit) Run() bool {
	madeProgress := false
	madeProgress = u.runWriteStage() || madeProgress
	madeProgress = u.runExecStage() || madeProgress
	madeProgress = u.runReadStage() || madeProgress
	return madeProgress
}

func (u *BranchUnit) runReadStage() bool {
	if u.state().Read == 0 {
		return false
	}

	if u.state().Exec == 0 {
		u.state().Exec = u.state().Read
		u.state().Read = 0

		return true
	}
	return false
}

func (u *BranchUnit) runExecStage() bool {
	if u.state().Exec == 0 {
		return false
	}

	if u.state().Write == 0 {
		// The ALU writes the PC only when the branch is taken, even if the
		// target is the next instruction.
		operation := u.cu.wave(u.state().Exec).emulation()
		u.alu.Run(operation)
		u.cu.comp.State.BranchTaken = operation.pcWritten

		u.state().Write = u.state().Exec
		u.state().Exec = 0
		return true
	}
	return false
}

func (u *BranchUnit) runWriteStage() bool {
	if u.state().Write == 0 {
		return false
	}

	wave := u.cu.wave(u.state().Write)
	u.cu.completeInstruction(wave, wave.InstID())

	if u.takenIssueGap > 0 && !u.cu.comp.State.BranchTaken {
		// Fetch continues in sequence, so the buffered instructions stay valid.
		u.cu.UpdatePCAndSetReady(wave)
	} else {
		wave.execution().InstBuffer = nil
		u.cu.UpdatePCAndSetReady(wave)
		wave.execution().InstBufferStartPC = wave.PC() & 0xffffffffffffffc0
		u.holdIssueAfterTakenBranch(wave)
	}

	u.state().Write = 0
	u.state().Idle = false
	return true
}

// holdIssueAfterTakenBranch keeps the wavefront from issuing until its SIMD's
// first turn after the configured gap since the branch was issued, and wakes
// the compute unit then.
func (u *BranchUnit) holdIssueAfterTakenBranch(wave *Wavefront) {
	if u.takenIssueGap <= 0 {
		return
	}

	gapEnd := u.cu.comp.Spec.Freq.NCyclesLater(
		u.takenIssueGap-branchPipelineCycles, u.cu.CurrentTime())
	state := wave.execution()
	state.IssueNotBefore = u.cu.nextIssueTurn(state.SIMDID, gapEnd)
	u.cu.comp.TickAt(state.IssueNotBefore)
}

func (u *BranchUnit) state() *StageState { return &u.cu.comp.State.Branch }
