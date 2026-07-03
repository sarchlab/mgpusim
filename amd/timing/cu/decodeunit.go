package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A DecodeUnit is any type of decode unit that takes one cycle to decode
type DecodeUnit struct {
	cu        *ComputeUnit
	ExecUnits []SubComponent // Execution units, index by SIMD number

	toDecode []*wavefront.Wavefront
	decoded  bool

	// stage names this decoder in the CU-qualified tracing location
	// "<cu>.<stage>" (the builder tags each decoder, e.g. "decode_scalar").
	stage string

	decodeTaskIDs map[uint64]uint64
	decodeDone    map[uint64]bool

	isIdle bool
}

// NewDecodeUnit creates a new decode unit
func NewDecodeUnit(cu *ComputeUnit) *DecodeUnit {
	du := new(DecodeUnit)
	du.cu = cu
	du.decoded = false
	du.stage = "decode"
	du.toDecode = make([]*wavefront.Wavefront, 0, 4)
	du.decodeTaskIDs = make(map[uint64]uint64)
	du.decodeDone = make(map[uint64]bool)
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
	return len(du.toDecode) < 4
}

// IsIdle checks idleness
func (du *DecodeUnit) IsIdle() bool {
	du.isIdle = (len(du.toDecode) == 0) && (du.decoded == false)
	return du.isIdle
}

// AcceptWave takes a wavefront and decode the instruction in the next cycle
func (du *DecodeUnit) AcceptWave(
	wave *wavefront.Wavefront,
) {
	if len(du.toDecode) >= 4 {
		log.Panicf("Decode unit busy, please run CanAcceptWave before accepting a wave")
	}

	du.toDecode = append(du.toDecode, wave)
	du.decoded = false
	du.startDecodeSubtask(wave.DynamicInst())
}

// Run decodes the instruction and sends the instruction to the next pipeline
// stage
func (du *DecodeUnit) Run() bool {
	madeProgress := false

	remaining := make([]*wavefront.Wavefront, 0, len(du.toDecode))
	for _, wave := range du.toDecode {
		simdID := wave.SIMDID
		execUnit := du.ExecUnits[simdID]

		if execUnit.CanAcceptWave() {
			inst := wave.DynamicInst()
			du.endDecodeSubtask(inst)
			if inst != nil && du.decodeDone[inst.ID] {
				du.markExecUnitReady(wave)
			}
			execUnit.AcceptWave(wave)
			if inst != nil {
				delete(du.decodeDone, inst.ID)
			}
			madeProgress = true
		} else {
			remaining = append(remaining, wave)
		}
	}
	du.toDecode = remaining

	if len(du.toDecode) > 0 && !du.decoded {
		for _, wave := range du.toDecode {
			inst := wave.DynamicInst()
			du.endDecodeSubtask(inst)
			if inst != nil {
				du.decodeDone[inst.ID] = true
			}
		}
		du.decoded = true
		return true
	}

	return madeProgress
}

func (du *DecodeUnit) startDecodeSubtask(inst *wavefront.Inst) {
	if inst == nil {
		return
	}

	taskID := timing.GetIDGenerator().Generate()
	tracing.StartTask(du.cu.comp, tracing.TaskStart{
		ID:       taskID,
		ParentID: inst.ID,
		Kind:     "pipeline",
		What:     du.cu.comp.Name() + "." + du.stage,
	})
	du.decodeTaskIDs[inst.ID] = taskID
}

func (du *DecodeUnit) endDecodeSubtask(inst *wavefront.Inst) {
	if inst == nil {
		return
	}

	taskID, ok := du.decodeTaskIDs[inst.ID]
	if !ok {
		return
	}

	tracing.EndTask(du.cu.comp, tracing.TaskEnd{ID: taskID})
	tracing.AddMilestone(du.cu.comp, tracing.Milestone{
		TaskID: inst.ID,
		Kind:   tracing.MilestoneKindWork,
		What:   du.cu.comp.Name() + "." + du.stage,
	})
	delete(du.decodeTaskIDs, inst.ID)
}

func (du *DecodeUnit) markExecUnitReady(wave *wavefront.Wavefront) {
	if wave.DynamicInst() == nil {
		return
	}

	tracing.AddMilestone(du.cu.comp, tracing.Milestone{
		TaskID: wave.DynamicInst().ID,
		Kind:   tracing.MilestoneKindHardwareResource,
		What: du.cu.comp.Name() + "." +
			du.cu.execUnitToString(wave.DynamicInst().ExeUnit),
	})
}

// Flush clear the unit
func (du *DecodeUnit) Flush() {
	for instID, taskID := range du.decodeTaskIDs {
		tracing.EndTask(du.cu.comp, tracing.TaskEnd{ID: taskID})
		tracing.AddMilestone(du.cu.comp, tracing.Milestone{
			TaskID: instID,
			Kind:   tracing.MilestoneKindWork,
			What:   "decode",
		})
		delete(du.decodeTaskIDs, instID)
	}
	du.decodeDone = make(map[uint64]bool)
	du.toDecode = du.toDecode[:0]
}
