package cu

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v3/emu"
	"github.com/sarchlab/mgpusim/v3/timing/wavefront"
)

// A SIMDUnit performs branch operations
type SIMDUnit struct {
	sim.HookableBase

	cu *ComputeUnit

	name string

	scratchpadPreparer ScratchpadPreparer
	alu                emu.ALU

	toExec    *wavefront.Wavefront
	cycleLeft int

	NumSinglePrecisionUnit int

	isIdle bool
}

// NewSIMDUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewSIMDUnit(
	cu *ComputeUnit,
	name string,
	scratchpadPreparer ScratchpadPreparer,
	alu emu.ALU,
) *SIMDUnit {
	u := new(SIMDUnit)
	u.name = name
	u.cu = cu
	u.scratchpadPreparer = scratchpadPreparer
	u.alu = alu

	u.NumSinglePrecisionUnit = 16

	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) CanAcceptWave() bool {
	return u.toExec == nil
}

// IsIdle checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) IsIdle() bool {
	u.isIdle = (u.toExec == nil)
	return u.isIdle
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *SIMDUnit) AcceptWave(wave *wavefront.Wavefront, now sim.VTimeInSec) {
	u.toExec = wave

	u.cycleLeft = 64 / u.NumSinglePrecisionUnit
	u.logPipelineTask(now, u.toExec.DynamicInst(), false)
}

// Run executes three pipeline stages that are controlled by the SIMDUnit
func (u *SIMDUnit) Run(now sim.VTimeInSec) bool {
	madeProgress := u.runExecStage(now)
	return madeProgress
}

func (u *SIMDUnit) runExecStage(now sim.VTimeInSec) bool {
	if u.toExec == nil {
		return false
	}

	u.cycleLeft--
	if u.cycleLeft > 0 {
		return true
	}

	u.scratchpadPreparer.Prepare(u.toExec, u.toExec)
	u.alu.Run(u.toExec)
	u.scratchpadPreparer.Commit(u.toExec, u.toExec)
	u.cu.UpdatePCAndSetReady(u.toExec)

	u.logPipelineTask(now, u.toExec.DynamicInst(), true)
	u.cu.logInstTask(now, u.toExec, u.toExec.DynamicInst(), true)

	u.toExec = nil
	return true
}

// Flush flushes
func (u *SIMDUnit) Flush() {
	u.toExec = nil
}

func (u *SIMDUnit) logPipelineTask(
	now sim.VTimeInSec,
	inst *wavefront.Inst,
	completed bool,
) {
	if completed {
		tracing.EndTask(
			inst.ID+"_simd_exec",
			u,
		)
		return
	}

	tracing.StartTask(
		inst.ID+"_simd_exec",
		inst.ID,
		u,
		"pipeline",
		u.cu.execUnitToString(inst.ExeUnit),
		// inst.InstName,
		nil,
	)
}

// Name names the unit
func (u *SIMDUnit) Name() string {
	return u.name
}
