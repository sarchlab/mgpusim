package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/timing/wavefront"
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
func taskTypeFromString(thisTask tracing.Task) (t taskType) {
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

func separateScalarTask(thisTask tracing.Task) (t taskType) {
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
//   - "scalar-mem": the wavefront is fetching an instruction and is waiting for
//     the scalar memory to be ready
//   - "vector-mem": the wavefront is fetching an instruction and is waiting for
//
// the vector memory to be ready
// - "lds": the wavefront is fetching an instruction and is waiting for the LDS
// to be ready
// - "scalar": the wavefront is executing a scalar instruction
// - "vector": the wavefront is executing a vector instruction
type CPIStackTracer struct {
	timeTeller sim.TimeTeller
	cu         *ComputeUnit

	inflightTasks        map[string]tracing.Task
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
	cu *ComputeUnit,
	timeTeller sim.TimeTeller,
) *CPIStackTracer {
	h := &CPIStackTracer{
		timeTeller: timeTeller,
		cu:         cu,

		inflightTasks: make(map[string]tracing.Task),
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
		endTime = float64(h.timeTeller.CurrentTime())
	}

	totalTime := endTime - h.firstWFStartTime
	totalCycle := totalTime * float64(h.cu.Freq)
	return totalCycle
}

func (h *CPIStackTracer) GetCPIStack() map[string]float64 {
	totalCycle := h.totalCycle()

	stack := make(map[string]float64)

	stack["total"] = totalCycle / float64(h.instCount)

	for taskType, duration := range h.timeStack {
		cycle := duration * float64(h.cu.Freq)
		stack[taskType] = cycle / float64(h.instCount)
	}

	return stack
}

func (h *CPIStackTracer) GetSIMDCPIStack() map[string]float64 {
	totalCycle := h.totalCycle()

	stack := make(map[string]float64)

	stack["total"] = totalCycle / float64(h.valuInstCount)

	for taskType, duration := range h.timeStack {
		cycle := duration * float64(h.cu.Freq)
		stack[taskType] = cycle / float64(h.valuInstCount)
	}

	return stack
}

// StartTask is called when a task is started.
func (h *CPIStackTracer) StartTask(task tracing.Task) {
	h.inflightTasks[task.ID] = task
	h.handleTaskStart(task)
}

// StepTask does nothing.
func (h *CPIStackTracer) StepTask(task tracing.Task) {
	// Do nothing
}

// AddMilestone does nothing.
func (h *CPIStackTracer) AddMilestone(milestone tracing.Milestone) {
	// Do nothing
}

// EndTask is called when a task is ended.
func (h *CPIStackTracer) EndTask(task tracing.Task) {
	originalTask, found := h.inflightTasks[task.ID]
	if found {
		delete(h.inflightTasks, task.ID)
		h.handleTaskEnd(originalTask)
	}
}

func (h *CPIStackTracer) handleTaskStart(task tracing.Task) {
	switch task.Kind {
	case "wavefront":
		if !h.firstWFStarted {
			h.firstWFStarted = true
			h.firstWFStartTime = float64(h.timeTeller.CurrentTime())
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

func (h *CPIStackTracer) handleRegularTaskStart(task tracing.Task) {
	currentTaskType := taskTypeFromString(task)
	highestTaskType := h.highestRunningTaskType()

	currentTime := h.timeTeller.CurrentTime()
	duration := h.timeDiff()
	h.timeStack[highestTaskType.ToString()] += duration
	h.lastRecordedTime = float64(currentTime)

	h.inFlightTaskCountMap[currentTaskType]++
}

func (h *CPIStackTracer) handleReqStart(task tracing.Task) {
	if task.What == "*mem.ReadReq" || task.What == "*mem.WriteReq" {
		parentTask, found := h.inflightTasks[task.ParentID]

		if !found {
			panic("Could not find parent task")
		}

		if parentTask.What == "VMem" {
			task.What = "VectorMemTransaction"
			h.handleRegularTaskStart(task)
		} else if parentTask.What == "Scalar" {
			task.What = "ScalarMemTransaction"
			h.handleRegularTaskStart(task)
		}
	}
}

func (h *CPIStackTracer) handleRegularTaskEnd(task tracing.Task) {
	currentTaskType := taskTypeFromString(task)
	highestTaskType := h.highestRunningTaskType()

	currentTime := h.timeTeller.CurrentTime()
	duration := h.timeDiff()

	h.timeStack[highestTaskType.ToString()] += duration
	h.lastRecordedTime = float64(currentTime)

	if currentTaskType.isInst() {
		h.instCount++
	}

	if currentTaskType == taskTypeVALU {
		h.valuInstCount++
	}

	h.inFlightTaskCountMap[currentTaskType]--
}

func (h *CPIStackTracer) handleReqEnd(task tracing.Task) {
	if task.What == "*mem.ReadReq" || task.What == "*mem.WriteReq" {
		parentTask, found := h.inflightTasks[task.ParentID]

		if !found {
			panic("Could not find parent task")
		}

		if parentTask.What == "VMem" {
			task.What = "VectorMemTransaction"
			h.handleRegularTaskEnd(task)
		} else if parentTask.What == "Scalar" {
			task.What = "ScalarMemTransaction"
			h.handleRegularTaskEnd(task)
		}
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

func (h *CPIStackTracer) timeDiff() float64 {
	return float64(h.timeTeller.CurrentTime()) - h.lastRecordedTime
}

func (h *CPIStackTracer) addStackTime(state string, time float64) {
	_, ok := h.timeStack[state]
	if !ok {
		h.timeStack[state] = 0
	}

	h.timeStack[state] += time
}

func (h *CPIStackTracer) handleTaskEnd(task tracing.Task) {
	switch task.Kind {
	case "wavefront":
		if h.firstWFStarted {
			h.lastWFEndTime = float64(h.timeTeller.CurrentTime())
			h.lastRecordedTime = h.lastWFEndTime
			h.runningWFCount--
		}
	case "inst", "fetch":
		h.handleRegularTaskEnd(task)
	case "req_out":
		h.handleReqEnd(task)
	}

	h.lastWFEndTime = float64(h.timeTeller.CurrentTime())
}
