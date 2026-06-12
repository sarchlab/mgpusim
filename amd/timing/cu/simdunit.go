package cu

import (
	"strings"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// simdPipelineSlot represents one in-flight wavefront in the SIMD pipeline.
type simdPipelineSlot struct {
	wf        *wavefront.Wavefront
	cycleLeft int
	taskID    uint64
}

// A SIMDUnit performs branch operations
type SIMDUnit struct {
	hooking.HookableBase

	cu *ComputeUnit

	name string

	alu emu.ALU

	toExec     *wavefront.Wavefront
	cycleLeft  int
	execTaskID uint64

	// Pipeline mode fields (used when scoreboardEnabled)
	pipelineSlots    []*simdPipelineSlot
	pipelineCapacity int

	scoreboardEnabled bool

	NumSinglePrecisionUnit int

	isIdle bool
}

// NewSIMDUnit creates a new branch unit, injecting the dependency of
// the compute unit.
func NewSIMDUnit(
	cu *ComputeUnit,
	name string,
	alu emu.ALU,
) *SIMDUnit {
	u := new(SIMDUnit)
	u.name = name
	u.cu = cu
	u.alu = alu

	u.NumSinglePrecisionUnit = 16

	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) CanAcceptWave() bool {
	if u.scoreboardEnabled {
		return len(u.pipelineSlots) < u.pipelineCapacity
	}
	return u.toExec == nil
}

// IsIdle checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) IsIdle() bool {
	if u.scoreboardEnabled {
		u.isIdle = len(u.pipelineSlots) == 0
		return u.isIdle
	}
	u.isIdle = (u.toExec == nil)
	return u.isIdle
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *SIMDUnit) AcceptWave(wave *wavefront.Wavefront) {
	cycleLeft := 64 / u.NumSinglePrecisionUnit
	if strings.Contains(wave.Inst().InstName, "f64") {
		cycleLeft = 64 / (u.NumSinglePrecisionUnit / 2)
	}

	if u.scoreboardEnabled {
		slot := &simdPipelineSlot{
			wf:        wave,
			cycleLeft: cycleLeft,
		}
		u.pipelineSlots = append(u.pipelineSlots, slot)
		slot.taskID = u.logPipelineTaskStart(wave.DynamicInst())
		return
	}

	u.toExec = wave
	u.cycleLeft = cycleLeft
	u.execTaskID = u.logPipelineTaskStart(u.toExec.DynamicInst())
}

// Run executes three pipeline stages that are controlled by the SIMDUnit
func (u *SIMDUnit) Run() bool {
	if u.scoreboardEnabled {
		return u.runPipelined()
	}
	return u.runExecStage()
}

func (u *SIMDUnit) runPipelined() bool {
	if len(u.pipelineSlots) == 0 {
		return false
	}

	madeProgress := false
	remaining := make([]*simdPipelineSlot, 0, len(u.pipelineSlots))

	for _, slot := range u.pipelineSlots {
		slot.cycleLeft--
		madeProgress = true

		if slot.cycleLeft <= 0 {
			u.alu.Run(slot.wf)
			u.cu.UpdatePCAndSetReady(slot.wf)

			u.logPipelineTaskEnd(slot.taskID)
			u.cu.logInstTask(slot.wf, slot.wf.DynamicInst(), true)
		} else {
			remaining = append(remaining, slot)
		}
	}

	u.pipelineSlots = remaining
	return madeProgress
}

func (u *SIMDUnit) runExecStage() bool {
	if u.toExec == nil {
		return false
	}

	u.cycleLeft--
	if u.cycleLeft > 0 {
		return true
	}

	u.alu.Run(u.toExec)
	u.cu.UpdatePCAndSetReady(u.toExec)

	u.logPipelineTaskEnd(u.execTaskID)
	u.cu.logInstTask(u.toExec, u.toExec.DynamicInst(), true)

	u.toExec = nil
	return true
}

// Flush flushes
func (u *SIMDUnit) Flush() {
	u.toExec = nil
	u.pipelineSlots = u.pipelineSlots[:0]
}

// logPipelineTaskStart starts the per-SIMD pipeline task for the given
// instruction and returns the task ID (v4 used the string ID
// inst.ID+"_simd_exec").
func (u *SIMDUnit) logPipelineTaskStart(inst *wavefront.Inst) uint64 {
	taskID := timing.GetIDGenerator().Generate()

	tracing.StartTask(u, tracing.TaskStart{
		ID:       taskID,
		ParentID: inst.ID,
		Kind:     "pipeline",
		What:     u.cu.execUnitToString(inst.ExeUnit),
	})

	return taskID
}

func (u *SIMDUnit) logPipelineTaskEnd(taskID uint64) {
	tracing.EndTask(u, tracing.TaskEnd{ID: taskID})
}

// Name names the unit
func (u *SIMDUnit) Name() string {
	return u.name
}

// CurrentTime returns the current time of the compute unit the SIMD unit
// belongs to. It makes the SIMD unit a tracing domain.
func (u *SIMDUnit) CurrentTime() timing.VTimeInPicoSec {
	return u.cu.comp.CurrentTime()
}
