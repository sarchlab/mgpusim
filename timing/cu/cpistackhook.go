package cu

import (
	"fmt"

	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
	"gitlab.com/akita/mgpusim/v3/timing/wavefront"
)

type taskType int

const (
	taskTypeIdle = iota
	taskTypeFetch
	taskTypeSpecial
	taskTypeVMem
	taskTypeLDS
	taskTypeBranch
	taskTypeScalar
	taskTypeVALU
	taskTypeCount
)

func (t taskType) ToString() string {
	switch t {
	case taskTypeIdle:
		return "idle"
	case taskTypeFetch:
		return "fetch"
	case taskTypeSpecial:
		return "Special"
	case taskTypeVMem:
		return "VMem"
	case taskTypeLDS:
		return "LDS"
	case taskTypeBranch:
		return "Branch"
	case taskTypeScalar:
		return "Scalar"
	case taskTypeVALU:
		return "VALU"
	default:
		return "unknown"
	}
}

func taskTypeFromString(s string) (t taskType) {
	switch s {
	case "idle":
		t = taskTypeIdle
	case "fetch":
		t = taskTypeFetch
	case "Special":
		t = taskTypeSpecial
	case "VMem":
		t = taskTypeVMem
	case "LDS":
		t = taskTypeLDS
	case "Branch":
		t = taskTypeBranch
	case "Scalar":
		t = taskTypeScalar
	case "VALU":
		t = taskTypeVALU
	default:
		panic("unknown task type " + s)
	}

	return
}

// A CPIStackInstHook is a hook to the CU that captures what instructions are
// issued in each cycle.
//
// The hook keep track of the state of the wavefronts. The state can be one of
// the following:
// - "idle": the wavefront is not doing anything
// - "fetch": the wavefront is fetching an instruction
// - "scalar-mem": the wavefront is fetching an instruction and is waiting for
//  the scalar memory to be ready
// - "vector-mem": the wavefront is fetching an instruction and is waiting for
// the vector memory to be ready
// - "lds": the wavefront is fetching an instruction and is waiting for the LDS
// to be ready
// - "scalar": the wavefront is executing a scalar instruction
// - "vector": the wavefront is executing a vector instruction
type CPIStackInstHook struct {
	timeTeller sim.TimeTeller
	cu         *ComputeUnit

	inflightTasks        map[string]tracing.Task
	firstWFStarted       bool
	firstWFStart         float64
	lastWFEnd            float64
	timeStack            map[string]float64
	lastRecordedTime     float64
	inFlightTaskCountMap map[taskType]uint64
	instCount            uint64
	valuInstCount        uint64
}

// NewCPIStackInstHook creates a CPIStackInstHook object.
func NewCPIStackInstHook(
	cu *ComputeUnit,
	timeTeller sim.TimeTeller,
) *CPIStackInstHook {
	h := &CPIStackInstHook{
		timeTeller: timeTeller,
		cu:         cu,

		inflightTasks: make(map[string]tracing.Task),
		timeStack:     make(map[string]float64),
		inFlightTaskCountMap: map[taskType]uint64{
			taskTypeIdle:    0,
			taskTypeFetch:   0,
			taskTypeSpecial: 0,
			taskTypeVMem:    0,
			taskTypeLDS:     0,
			taskTypeBranch:  0,
			taskTypeScalar:  0,
			taskTypeVALU:    0,
		},
	}

	atexit.Register(func() {
		h.Report()
	})

	return h
}

// Report reports the data collected.
func (h *CPIStackInstHook) Report() {
	totalTime := h.lastWFEnd - h.firstWFStart

	if totalTime == 0 {
		return
	}

	cpi := totalTime * float64(h.cu.Freq) / float64(h.instCount)
	simdCPI := totalTime * float64(h.cu.Freq) / float64(h.valuInstCount)
	fmt.Printf("%s, CPI, %f\n", h.cu.Name(), cpi)
	fmt.Printf("%s, SIMD CPI: %f\n", h.cu.Name(), simdCPI)

	for taskType, duration := range h.timeStack {
		cpi := duration * float64(h.cu.Freq) / float64(h.instCount)
		simdCPI := duration * float64(h.cu.Freq) / float64(h.valuInstCount)

		fmt.Printf("%s, %s, %.10f\n",
			h.cu.Name(), "CPIStack."+taskType, cpi)
		fmt.Printf("%s, %s, %.10f\n",
			h.cu.Name(), "SIMDCPIStack."+taskType, simdCPI)
	}
}

// Func records issued instructions.
func (h *CPIStackInstHook) Func(ctx sim.HookCtx) {
	switch ctx.Pos {
	case tracing.HookPosTaskStart:
		task := ctx.Item.(tracing.Task)
		h.inflightTasks[task.ID] = task
		h.handleTaskStart(task)
	case tracing.HookPosTaskEnd:
		task := ctx.Item.(tracing.Task)
		originalTask, found := h.inflightTasks[task.ID]
		if found {
			delete(h.inflightTasks, task.ID)
			h.handleTaskEnd(originalTask)
		}
	default:
		return
	}
}

func (h *CPIStackInstHook) handleTaskStart(task tracing.Task) {
	switch task.Kind {
	case "wavefront":
		if !h.firstWFStarted {
			h.firstWFStarted = true
			h.firstWFStart = float64(h.timeTeller.CurrentTime())
			h.lastRecordedTime = h.firstWFStart
		}
	case "inst", "fetch":
		h.handleRegularTaskStart(task)
	default:
		fmt.Println("Unknown task kind:", task.Kind, task.What)
	}
}

func (h *CPIStackInstHook) handleRegularTaskStart(task tracing.Task) {
	currentTaskType := taskTypeFromString(task.What)

	highestTaskType := h.highestRunningTaskType()

	currentTime := h.timeTeller.CurrentTime()
	duration := h.timeDiff()
	h.timeStack[highestTaskType.ToString()] += duration
	h.lastRecordedTime = float64(currentTime)

	h.inFlightTaskCountMap[currentTaskType]++

	fmt.Printf("%.10f, %s, start task, %s, %s, %.10f\n",
		currentTime, h.cu.Name(),
		currentTaskType.ToString(), highestTaskType.ToString(),
		duration)

	if currentTaskType == taskTypeScalar {
		detail := task.Detail.(map[string]interface{})
		inst := detail["inst"].(*wavefront.Inst)
		fmt.Println(inst.FormatName)
	}
}

func (h *CPIStackInstHook) handleRegularTaskEnd(task tracing.Task) {
	currentTaskType := taskTypeFromString(task.What)

	currentTime := h.timeTeller.CurrentTime()
	duration := h.timeDiff()
	highestTaskType := h.highestRunningTaskType()
	h.timeStack[highestTaskType.ToString()] += duration
	h.lastRecordedTime = float64(currentTime)

	if currentTaskType == taskTypeLDS || currentTaskType == taskTypeVMem ||
		currentTaskType == taskTypeBranch || currentTaskType == taskTypeScalar || currentTaskType == taskTypeVALU {
		h.instCount++
	}

	if currentTaskType == taskTypeVALU {
		h.valuInstCount++
	}

	h.inFlightTaskCountMap[currentTaskType]--

	// fmt.Printf("%.10f, %s, end task, %s, %s, %.10f\n",
	// 	currentTime, h.cu.Name(),
	// 	currentTaskType.ToString(), highestTaskType.ToString(),
	// 	duration)
}

func (h *CPIStackInstHook) highestRunningTaskType() taskType {
	for t := taskType(taskTypeCount) - 1; t > taskTypeIdle; t-- {
		if h.inFlightTaskCountMap[t] > 0 {
			return t
		}
	}

	return taskTypeIdle
}

func (h *CPIStackInstHook) timeDiff() float64 {
	return float64(h.timeTeller.CurrentTime()) - h.lastRecordedTime
}

func (h *CPIStackInstHook) addStackTime(state string, time float64) {
	_, ok := h.timeStack[state]
	if !ok {
		h.timeStack[state] = 0
	}

	h.timeStack[state] += time
}

func (h *CPIStackInstHook) handleTaskEnd(task tracing.Task) {
	switch task.Kind {
	case "wavefront":
		if h.firstWFStarted {
			h.lastWFEnd = float64(h.timeTeller.CurrentTime())
			h.lastRecordedTime = h.lastWFEnd
		}
	case "inst", "fetch":
		h.handleRegularTaskEnd(task)
	}

	h.lastWFEnd = float64(h.timeTeller.CurrentTime())

}
