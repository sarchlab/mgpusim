package cu

import (
	"strings"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A SIMDUnit performs branch operations
type SIMDUnit struct {
	hooking.HookableBase

	cu    *ComputeUnit
	index int

	name string

	alu emu.ALU

	// Pipeline mode fields (used when scoreboardEnabled)
	pipelineCapacity int

	scoreboardEnabled bool

	NumSinglePrecisionUnit int
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
		return len(u.state().Slots) < u.pipelineCapacity
	}
	return u.state().Exec == 0
}

// IsIdle checks if the buffer of the read stage is occupied or not
func (u *SIMDUnit) IsIdle() bool {
	if u.scoreboardEnabled {
		u.state().Idle = len(u.state().Slots) == 0
		return u.state().Idle
	}
	u.state().Idle = (u.state().Exec == 0)
	return u.state().Idle
}

// AcceptWave moves one wavefront into the read buffer of the branch unit
func (u *SIMDUnit) AcceptWave(wave *wavefront.Wavefront) {
	cycleLeft := 64 / u.NumSinglePrecisionUnit
	if strings.Contains(wave.Inst().InstName, "f64") {
		cycleLeft = 64 / (u.NumSinglePrecisionUnit / 2)
	}

	if u.scoreboardEnabled {
		slot := SIMDSlotState{
			WF:     u.cu.useWave(wave),
			Cycles: cycleLeft,
		}
		u.state().Slots = append(u.state().Slots, slot)
		u.state().Slots[len(u.state().Slots)-1].Task = u.logPipelineTaskStart(wave.DynamicInst())
		return
	}

	u.state().Exec = u.cu.useWave(wave)
	u.state().Cycles = cycleLeft
	u.state().Task = u.logPipelineTaskStart(u.cu.wave(u.state().Exec).DynamicInst())
}

// Run executes three pipeline stages that are controlled by the SIMDUnit
func (u *SIMDUnit) Run() bool {
	if u.scoreboardEnabled {
		return u.runPipelined()
	}
	return u.runExecStage()
}

func (u *SIMDUnit) runPipelined() bool {
	if len(u.state().Slots) == 0 {
		return false
	}

	madeProgress := false
	remaining := make([]SIMDSlotState, 0, len(u.state().Slots))

	for _, slot := range u.state().Slots {
		slot.Cycles--
		madeProgress = true

		if slot.Cycles <= 0 {
			u.alu.Run(u.cu.wave(slot.WF))
			u.cu.UpdatePCAndSetReady(u.cu.wave(slot.WF))

			u.logPipelineTaskEnd(slot.Task)
			u.cu.logInstTask(u.cu.wave(slot.WF), u.cu.wave(slot.WF).DynamicInst(), true)
		} else {
			remaining = append(remaining, slot)
		}
	}

	u.state().Slots = remaining
	return madeProgress
}

func (u *SIMDUnit) runExecStage() bool {
	if u.state().Exec == 0 {
		return false
	}

	u.state().Cycles--
	if u.state().Cycles > 0 {
		return true
	}

	u.alu.Run(u.cu.wave(u.state().Exec))
	u.cu.UpdatePCAndSetReady(u.cu.wave(u.state().Exec))

	u.logPipelineTaskEnd(u.state().Task)
	u.cu.logInstTask(u.cu.wave(u.state().Exec), u.cu.wave(u.state().Exec).DynamicInst(), true)

	u.state().Exec = 0
	return true
}

// Flush flushes
func (u *SIMDUnit) Flush() {
	// End the per-SIMD "pipeline" tasks of the instructions dropped from the
	// pipeline, so they do not leak as started-never-ended. (The instructions'
	// parent "inst" tasks are ended by ComputeUnit.endInflightTracingTasks.)
	if u.state().Exec != 0 {
		u.cu.comp.TaskTracker().EndTask(u.state().Task)
	}
	for _, slot := range u.state().Slots {
		u.cu.comp.TaskTracker().EndTask(slot.Task)
	}

	u.state().Exec = 0
	u.state().Slots = u.state().Slots[:0]
}

// logPipelineTaskStart starts the per-SIMD pipeline task for the given
// instruction and returns the task ID (v4 used the string ID
// inst.ID+"_simd_exec").
//
// The task is emitted on the SIMD unit itself (domain u), not on cu.comp, on
// purpose: report.go attaches a per-SIMD BusyTimeTracer to each SIMD unit
// filtered to Kind=="pipeline" to measure per-SIMD busy time, which requires
// the task to live on that unit's own tracing domain. Its parent ("inst")
// lives on cu.comp; that cross-domain parent link still resolves because the
// vis DBTracer is the same instance on both domains. Do not move this to
// cu.comp — it would collapse the per-SIMD busy-time metric.
func (u *SIMDUnit) logPipelineTaskStart(inst *wavefront.Inst) uint64 {
	taskID := u.cu.comp.TaskTracker().StartTask(u, tasks.StartInfo{
		ParentID: u.cu.comp.State.InstTasks[inst.ID],
		Kind:     "pipeline",
		What:     u.Name() + "." + u.cu.execUnitToString(inst.ExeUnit),
	})

	return taskID
}

func (u *SIMDUnit) logPipelineTaskEnd(taskID uint64) {
	u.cu.comp.TaskTracker().EndTask(taskID)
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

func (u *SIMDUnit) TaskTracker() *tasks.Tracker { return u.cu.comp.TaskTracker() }

func (u *SIMDUnit) state() *SIMDState { return &u.cu.comp.State.SIMDs[u.index] }
