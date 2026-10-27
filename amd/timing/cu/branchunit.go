package cu

import (
	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

// A BranchUnit performs branch operations
type BranchUnit struct {
	cu *ComputeUnit

	alu emu.ALU
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
		u.alu.Run(u.cu.wave(u.state().Exec).emulation())

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

	u.cu.completeInstruction(u.cu.wave(u.state().Write), u.cu.wave(u.state().Write).InstID())

	u.cu.wave(u.state().Write).execution().InstBuffer = nil
	u.cu.UpdatePCAndSetReady(u.cu.wave(u.state().Write))
	u.cu.wave(u.state().Write).execution().InstBufferStartPC = u.cu.wave(u.state().Write).PC() & 0xffffffffffffffc0
	u.state().Write = 0
	u.state().Idle = false
	return true
}

func (u *BranchUnit) state() *StageState { return &u.cu.comp.State.Branch }
