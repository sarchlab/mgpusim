package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A DecodeUnit is any type of decode unit that takes one cycle to decode
type DecodeUnit struct {
	cu        *ComputeUnit
	ExecUnits []SubComponent // Execution units, index by SIMD number

	index int

	// stage names this decoder in the CU-qualified tracing location
	// "<cu>.<stage>" (the builder tags each decoder, e.g. "decode_scalar").
	stage string
}

// NewDecodeUnit creates a new decode unit
func NewDecodeUnit(cu *ComputeUnit) *DecodeUnit {
	du := new(DecodeUnit)
	du.cu = cu
	du.stage = "decode"
	return du
}

// AddExecutionUnit registers an executions unit to the decode unit, so that
// the decode unit knows where to send the instruction to after decoding.
// This function has to be called in the order of SIMD number.
func (du *DecodeUnit) AddExecutionUnit(cuComponent SubComponent) {
	du.ExecUnits = append(du.ExecUnits, cuComponent)
}

// CanAcceptWave checks if the DecodeUnit is ready to decode another
// instruction
func (du *DecodeUnit) CanAcceptWave() bool {
	return len(du.state().ToDecode) < 4
}

// IsIdle checks idleness
func (du *DecodeUnit) IsIdle() bool {
	du.state().Idle = (len(du.state().ToDecode) == 0) && (du.state().Decoded == false)
	return du.state().Idle
}

// AcceptWave takes a wavefront and decode the instruction in the next cycle
func (du *DecodeUnit) AcceptWave(
	wave *Wavefront,
) {
	if len(du.state().ToDecode) >= 4 {
		log.Panicf("Decode unit busy, please run CanAcceptWave before accepting a wave")
	}

	du.state().ToDecode = append(du.state().ToDecode, du.cu.useWave(wave))
	du.state().Decoded = false
	du.startDecodeSubtask(wave.DynamicInst())
}

// Run decodes the instruction and sends the instruction to the next pipeline
// stage
func (du *DecodeUnit) Run() bool {
	madeProgress := false

	remaining := make([]uint64, 0, len(du.state().ToDecode))
	for _, id := range du.state().ToDecode {
		wave := du.cu.wave(id)
		simdID := wave.execution().SIMDID
		execUnit := du.ExecUnits[simdID]

		if execUnit.CanAcceptWave() {
			inst := wave.DynamicInst()
			du.endDecodeSubtask(inst)
			if inst != nil && du.state().Done[inst.ID] {
				du.markExecUnitReady(wave)
			}
			execUnit.AcceptWave(wave)
			if inst != nil {
				delete(du.state().Done, inst.ID)
			}
			madeProgress = true
		} else {
			remaining = append(remaining, id)
		}
	}
	du.state().ToDecode = remaining

	if len(du.state().ToDecode) > 0 && !du.state().Decoded {
		for _, id := range du.state().ToDecode {
			wave := du.cu.wave(id)
			inst := wave.DynamicInst()
			du.endDecodeSubtask(inst)
			if inst != nil {
				du.state().Done[inst.ID] = true
			}
		}
		du.state().Decoded = true
		return true
	}

	return madeProgress
}

func (du *DecodeUnit) startDecodeSubtask(inst *wavefront.Inst) {
	if inst == nil {
		return
	}

	taskID := du.cu.comp.TaskTracker().StartTask(du.cu.comp, tasks.StartInfo{
		ParentID: du.cu.comp.State.InstTasks[inst.ID],
		Kind:     "pipeline",
		What:     du.cu.comp.Name() + "." + du.stage,
	})
	du.state().Tasks[inst.ID] = taskID
}

func (du *DecodeUnit) endDecodeSubtask(inst *wavefront.Inst) {
	if inst == nil {
		return
	}

	taskID, ok := du.state().Tasks[inst.ID]
	if !ok {
		return
	}

	du.cu.comp.TaskTracker().EndTask(taskID)
	du.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: du.cu.comp.State.InstTasks[inst.ID],
		Kind:   tasks.MilestoneKindWork,
		What:   du.cu.comp.Name() + "." + du.stage,
	})
	delete(du.state().Tasks, inst.ID)
}

func (du *DecodeUnit) markExecUnitReady(wave *Wavefront) {
	if wave.DynamicInst() == nil {
		return
	}

	du.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: du.cu.comp.State.InstTasks[wave.DynamicInst().ID],
		Kind:   tasks.MilestoneKindHardwareResource,
		What: du.cu.comp.Name() + "." +
			du.cu.execUnitToString(wave.DynamicInst().ExeUnit),
	})
}

// Flush clear the unit
func (du *DecodeUnit) Flush() {
	for instID, taskID := range du.state().Tasks {
		du.cu.comp.TaskTracker().EndTask(taskID)
		du.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
			TaskID: du.cu.comp.State.InstTasks[instID],
			Kind:   tasks.MilestoneKindWork,
			What:   "decode",
		})
		delete(du.state().Tasks, instID)
	}
	du.state().Done = make(map[uint64]bool)
	du.state().ToDecode = du.state().ToDecode[:0]
}

func (du *DecodeUnit) state() *DecoderState { return &du.cu.comp.State.Decoders[du.index] }
