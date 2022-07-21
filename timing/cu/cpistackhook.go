package cu

import (
	"fmt"

	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
)

type taskType int

const (
	taskTypeIdle = iota
	taskTypeFetch
	taskTypeSpecial
	taskTypeVMem
	taskTypeLDS
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

	inflightTasks     map[string]tracing.Task
	inflightWfs       map[string]tracing.Task
	firstWFStarted    bool
	firstWFStart      float64
	lastWFEnd         float64
	timeStack         map[string]float64
	lastRecordedTime  float64
	totalInFlightTask uint64

	simdInstCount        uint64
	allInstCount         uint64
	inFlightTaskCountMap map[taskType]uint64
	taskCaseMap          map[string]float64
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
		inflightWfs:   make(map[string]tracing.Task),
		timeStack:     make(map[string]float64),
		inFlightTaskCountMap: map[taskType]uint64{
			taskTypeIdle:    0,
			taskTypeFetch:   0,
			taskTypeSpecial: 0,
			taskTypeVMem:    0,
			taskTypeLDS:     0,
			taskTypeScalar:  0,
			taskTypeVALU:    0,
		},

		taskCaseMap: map[string]float64{
			"case0": 0,
			"case1": 0,
			"case2": 0,
			"case3": 0,
			"case4": 0,
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

	for taskType, duration := range h.timeStack {
		fmt.Printf("%s, %s, %.10f\n",
			h.cu.Name(), taskType, duration*float64(h.cu.Freq))
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

	// cu := ctx.Domain.(*ComputeUnit)
	// task := ctx.Item.(tracing.Task)

	// switch task.Kind {

	// }

	// // fmt.Printf("%.10f, %s, %s\n",
	// // 	h.timeTeller.CurrentTime(), cu.Name(), ctx.Pos.Name)

	// fmt.Printf("\tTask %s-%s starts\n", task.Kind, task.What)
}

func (h *CPIStackInstHook) handleTaskStart(task tracing.Task) {
	// fmt.Printf("%.10f, %s, %s-%s\n",
	// h.timeTeller.CurrentTime(), h.cu.Name(), task.Kind, task.What)

	h.handleInFlightInst(task, true)

	switch task.Kind {
	case "wavefront":
		h.inflightWfs[task.ID] = task
		if !h.firstWFStarted {
			h.firstWFStarted = true
			h.firstWFStart = float64(h.timeTeller.CurrentTime())
			h.lastRecordedTime = h.firstWFStart
		}
	case "inst", "fetch":
		h.handleRegularTaskStart(task)
	}

	h.handleCaseChange(task, true)
	h.handleCurrentState(task)
}

func (h *CPIStackInstHook) handleRegularTaskStart(task tracing.Task) {
	h.allInstCount++

	taskType := taskTypeFromString(task.What)

	highestTaskType := h.highestRunningTaskType()

	h.timeStack[highestTaskType.ToString()] += h.timeDiff()
	h.lastRecordedTime = float64(h.timeTeller.CurrentTime())

	fmt.Printf("Starting an instruction of type %s, highest task type is %s\n",
		taskType.ToString(), highestTaskType.ToString())

	h.inFlightTaskCountMap[taskType] += 1
}

func (h *CPIStackInstHook) highestRunningTaskType() taskType {
	for t := taskType(taskTypeCount) - 1; t > taskTypeIdle; t-- {
		if h.inFlightTaskCountMap[t] > 0 {
			return t
		}
	}

	return taskTypeIdle
}

func (h *CPIStackInstHook) handleFetchStart(task tracing.Task) {
	// switch h.state {
	// case "idle":
	// 	h.state = "fetch"
	// 	h.addStackTime("idle", h.timeDiff())
	// }
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

func (h *CPIStackInstHook) handleInFlightInst(task tracing.Task, beginning bool) {
	// if task.What != "*mem.ReadReq" && task.What != "*mem.WriteReq" &&
	// 	task.What != "*protocol.MapWGReq" && task.What != "wavefront" {
	// 	if beginning {
	// 		h.inFlightTaskCountMap[task.What]++
	// 		h.totalInFlightTask++
	// 	} else {
	// 		h.inFlightTaskCountMap[task.What]--
	// 		h.totalInFlightTask--
	// 	}
	// }
}

func (h *CPIStackInstHook) handleCurrentState(task tracing.Task) {
	// h.state = "idle"

	// for k := range h.inFlightInstMap {
	// 	if h.taskHierarchy[h.state] < h.taskHierarchy[k] && h.inFlightInstMap[k] != 0 {
	// 		h.state = k
	// 	}
	// 	fmt.Printf("%s: %d\n", k, h.inFlightInstMap[k])
	// }

	// fmt.Printf("TOTAL INST COUNT: %d\nFINAL STATE: %s\n\n", h.totalInFlightTask, h.state)
}

func (h *CPIStackInstHook) handleCaseChange(task tracing.Task, beginning bool) {
	// if beginning {
	// 	if h.totalInFlightTask == 1 {
	// 		h.taskCaseMap["case0"] += h.timeDiff()
	// 	} else if h.taskHierarchy[task.What] > h.taskHierarchy[h.state] {
	// 		h.taskCaseMap["case1"] += h.timeDiff()
	// 	} else if h.taskHierarchy[task.What] < h.taskHierarchy[h.state] {
	// 		h.taskCaseMap["case2"] += h.timeDiff()
	// 	}
	// } else {
	// 	if h.taskHierarchy[h.state] == h.taskHierarchy[task.What] {
	// 		h.handleCurrentState(task)
	// 		h.taskCaseMap["case3"] += h.timeDiff()
	// 	} else {
	// 		h.taskCaseMap["case4"] += h.timeDiff()
	// 	}
	// }
}

func (h *CPIStackInstHook) handleTaskEnd(task tracing.Task) {
	_, ok := h.inflightWfs[task.ID]

	h.handleInFlightInst(task, false)
	h.handleCaseChange(task, false)

	if ok {
		delete(h.inflightWfs, task.ID)
		h.lastWFEnd = float64(h.timeTeller.CurrentTime())
	}
}
