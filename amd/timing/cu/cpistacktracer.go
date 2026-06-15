package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type taskType int

const (
	taskTypeIdle = iota
	taskTypeFetch
	taskTypeSpecial
	taskTypeVMemInst
	taskTypeScalarMemInst
	taskTypeVMem
	taskTypeScalarMem
	taskTypeLDS
	taskTypeBranch
	taskTypeScalarInst
	taskTypeVALU
	taskTypeCount
)

func (t taskType) isInst() bool {
	switch t {
	case taskTypeSpecial,
		taskTypeVMemInst,
		taskTypeScalarInst,
		taskTypeScalarMemInst,
		taskTypeLDS,
		taskTypeBranch,
		taskTypeVALU:
		return true
	}

	return false
}

//nolint:gocyclo
func (t taskType) ToString() string {
	switch t {
	case taskTypeIdle:
		return "Idle"
	case taskTypeFetch:
		return "Fetch"
	case taskTypeSpecial:
		return "Special"
	case taskTypeVMem:
		return "VMem"
	case taskTypeVMemInst:
		return "VMemInst"
	case taskTypeScalarMem:
		return "ScalarMem"
	case taskTypeScalarMemInst:
		return "ScalarMemInst"
	case taskTypeLDS:
		return "LDS"
	case taskTypeBranch:
		return "Branch"
	case taskTypeScalarInst:
		return "ScalarInst"
	case taskTypeVALU:
		return "VALU"
	default:
		return "unknown"
	}
}

//nolint:gocyclo
func taskTypeFromString(thisTask tracing.TaskStart) (t taskType) {
	switch thisTask.What {
	case "idle":
		t = taskTypeIdle
	case "fetch":
		t = taskTypeFetch
	case "Special":
		t = taskTypeSpecial
	case "VMem":
		t = taskTypeVMemInst
	case "LDS":
		t = taskTypeLDS
	case "Branch":
		t = taskTypeBranch
	case "Scalar":
		t = separateScalarTask(thisTask)
	case "VALU":
		t = taskTypeVALU
	case "ScalarMemTransaction":
		t = taskTypeScalarMem
	case "VectorMemTransaction":
		t = taskTypeVMem
	default:
		panic("unknown task type " + thisTask.What)
	}

	return
}

func separateScalarTask(thisTask tracing.TaskStart) (t taskType) {
	detail := thisTask.Detail.(map[string]interface{})
	inst := detail["inst"].(*wavefront.Inst)

	if inst.FormatName == "smem" {
		return taskTypeScalarMemInst
	}

	return taskTypeScalarInst
}

// A CPIStackTracer is a hook to the CU that captures what instructions are
// issued in each cycle.
//
// The hook keep track of the state of the wavefronts. The state can be one of
// the following:
//   - "idle": the wavefront is not doing anything
//   - "fetch": the wavefront is fetching an instruction
//   - "scalar-mem": the wavefront is fetching an instruction and is waiting
//     for the scalar memory to be ready
//   - "vector-mem": the wavefront is fetching an instruction and is waiting
//     for the vector memory to be ready
//   - "lds": the wavefront is fetching an instruction and is waiting for the
//     LDS to be ready
//   - "scalar": the wavefront is executing a scalar instruction
//   - "vector": the wavefront is executing a vector instruction
type CPIStackTracer struct {
	timeTeller timing.TimeTeller
	cu         *Comp

	inflightTasks        map[uint64]tracing.TaskStart
	firstWFStarted       bool
	firstWFStartTime     float64
	lastWFEndTime        float64
	timeStack            map[string]float64
	lastRecordedTime     float64
	inFlightTaskCountMap map[taskType]uint64
	instCount            uint64
	valuInstCount        uint64
	runningWFCount       uint64
}

// NewCPIStackInstHook creates a CPIStackInstHook object.
func NewCPIStackInstHook(
	cu *Comp,
	timeTeller timing.TimeTeller,
) *CPIStackTracer {
	h := &CPIStackTracer{
		timeTeller: timeTeller,
		cu:         cu,

		inflightTasks: make(map[uint64]tracing.TaskStart),
		timeStack:     make(map[string]float64),
		inFlightTaskCountMap: map[taskType]uint64{
			taskTypeIdle:          0,
			taskTypeFetch:         0,
			taskTypeSpecial:       0,
			taskTypeVMemInst:      0,
			taskTypeVMem:          0,
			taskTypeLDS:           0,
			taskTypeBranch:        0,
			taskTypeScalarInst:    0,
			taskTypeScalarMemInst: 0,
			taskTypeScalarMem:     0,
			taskTypeVALU:          0,
		},
	}

	return h
}

func (h *CPIStackTracer) totalCycle() float64 {
	endTime := h.lastWFEndTime
	if h.runningWFCount > 0 {
		endTime = secondsOf(h.timeTeller.CurrentTime())
	}

	totalTime := endTime - h.firstWFStartTime
	totalCycle := totalTime * float64(h.cu.Spec().Freq)
	return totalCycle
}

// GetCPIStack returns the CPI stack considering all the instructions.
func (h *CPIStackTracer) GetCPIStack() map[string]float64 {
	totalCycle := h.totalCycle()

	stack := make(map[string]float64)

	stack["total"] = totalCycle / float64(h.instCount)

	for taskType, duration := range h.timeStack {
		cycle := duration * float64(h.cu.Spec().Freq)
		stack[taskType] = cycle / float64(h.instCount)
	}

	return stack
}

// GetSIMDCPIStack returns the CPI stack considering only the VALU
// instructions.
func (h *CPIStackTracer) GetSIMDCPIStack() map[string]float64 {
	totalCycle := h.totalCycle()

	stack := make(map[string]float64)

	stack["total"] = totalCycle / float64(h.valuInstCount)

	for taskType, duration := range h.timeStack {
		cycle := duration * float64(h.cu.Spec().Freq)
		stack[taskType] = cycle / float64(h.valuInstCount)
	}

	return stack
}

// StartTask is called when a task is started.
func (h *CPIStackTracer) StartTask(task tracing.TaskStart) {
	h.inflightTasks[task.ID] = task
	h.handleTaskStart(task)
}

// AddTaskTag does nothing.
func (h *CPIStackTracer) AddTaskTag(tag tracing.TaskTag) {
	// Do nothing
}

// AddMilestone does nothing.
func (h *CPIStackTracer) AddMilestone(milestone tracing.Milestone) {
	// Do nothing
}

// EndTask is called when a task is ended.
func (h *CPIStackTracer) EndTask(task tracing.TaskEnd) {
	originalTask, found := h.inflightTasks[task.ID]
	if found {
		delete(h.inflightTasks, task.ID)
		h.handleTaskEnd(originalTask, task.Time)
	}
}

func (h *CPIStackTracer) handleTaskStart(task tracing.TaskStart) {
	switch task.Kind {
	case "wavefront":
		if !h.firstWFStarted {
			h.firstWFStarted = true
			h.firstWFStartTime = secondsOf(task.Time)
			h.lastRecordedTime = h.firstWFStartTime
			h.runningWFCount++
		}
	case "inst", "fetch":
		h.handleRegularTaskStart(task)
	case "req_out":
		h.handleReqStart(task)
	case "req_in":
		return
	default:
		fmt.Println("Unknown task kind:", task.Kind, task.What)
	}
}

func (h *CPIStackTracer) handleRegularTaskStart(task tracing.TaskStart) {
	currentTaskType := taskTypeFromString(task)
	highestTaskType := h.highestRunningTaskType()

	currentTime := secondsOf(task.Time)
	duration := currentTime - h.lastRecordedTime
	h.timeStack[highestTaskType.ToString()] += duration
	h.lastRecordedTime = currentTime

	h.inFlightTaskCountMap[currentTaskType]++
}

func (h *CPIStackTracer) handleReqStart(task tracing.TaskStart) {
	if task.What != "ReadReq" && task.What != "WriteReq" {
		return
	}

	parentTask, found := h.inflightTasks[task.ParentID]
	if !found {
		// The parent instruction (or fetch) task has already ended. With
		// coalesced vector-memory accesses, the requests for one instruction
		// share its task as their parent, and the instruction is completed
		// when the coalesce-terminating request returns. Because the memory
		// system may return responses out of order, a sibling request can
		// return after the instruction task has already ended. Such a
		// request cannot be attributed, so skip it; handleReqEnd is
		// symmetric and only accounts for requests classified here.
		return
	}

	switch parentTask.What {
	case "VMem":
		task.What = "VectorMemTransaction"
		h.inflightTasks[task.ID] = task
		h.handleRegularTaskStart(task)
	case "Scalar":
		task.What = "ScalarMemTransaction"
		h.inflightTasks[task.ID] = task
		h.handleRegularTaskStart(task)
	}
}

func (h *CPIStackTracer) handleRegularTaskEnd(
	task tracing.TaskStart,
	endTime timing.VTimeInPicoSec,
) {
	currentTaskType := taskTypeFromString(task)
	highestTaskType := h.highestRunningTaskType()

	currentTime := secondsOf(endTime)
	duration := currentTime - h.lastRecordedTime

	h.timeStack[highestTaskType.ToString()] += duration
	h.lastRecordedTime = currentTime

	if currentTaskType.isInst() {
		h.instCount++
	}

	if currentTaskType == taskTypeVALU {
		h.valuInstCount++
	}

	h.inFlightTaskCountMap[currentTaskType]--
}

func (h *CPIStackTracer) handleReqEnd(
	task tracing.TaskStart,
	endTime timing.VTimeInPicoSec,
) {
	// task is the request's own task, as persisted by handleReqStart. If the
	// request was classified as a memory transaction under a VMem/Scalar
	// instruction, its What was rewritten accordingly at start; account for
	// it symmetrically here. Requests that were not classified (the parent
	// instruction task was already gone at start) are skipped, exactly as at
	// start, so the in-flight counters stay balanced.
	switch task.What {
	case "VectorMemTransaction", "ScalarMemTransaction":
		h.handleRegularTaskEnd(task, endTime)
	}
}

func (h *CPIStackTracer) highestRunningTaskType() taskType {
	for t := taskType(taskTypeCount) - 1; t > taskTypeIdle; t-- {
		if h.inFlightTaskCountMap[t] > 0 {
			return t
		}
	}

	return taskTypeIdle
}

func (h *CPIStackTracer) handleTaskEnd(
	task tracing.TaskStart,
	endTime timing.VTimeInPicoSec,
) {
	switch task.Kind {
	case "wavefront":
		if h.firstWFStarted {
			h.lastWFEndTime = secondsOf(endTime)
			h.lastRecordedTime = h.lastWFEndTime
			h.runningWFCount--
		}
	case "inst", "fetch":
		h.handleRegularTaskEnd(task, endTime)
	case "req_out":
		h.handleReqEnd(task, endTime)
	}

	h.lastWFEndTime = secondsOf(endTime)
}
