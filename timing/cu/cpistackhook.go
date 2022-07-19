package cu

import (
	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
)

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

	state             string
	inflightWfs       map[string]tracing.Task
	firstWFStarted    bool
	firstWFStart      float64
	lastWFEnd         float64
	timeStack         map[string]float64
	lastTaskTime      float64
	greatestTask      string
	totalInFlightInst uint64

	simdInstCount         uint64
	allInstCount          uint64
	inFlightInstBreakdown map[string]uint64
	instChangeBreakdown   map[string]float64

	taskHierarchy map[string]uint64
}

// NewCPIStackInstHook creates a CPIStackInstHook object.
func NewCPIStackInstHook(cu *ComputeUnit, timeTeller sim.TimeTeller) *CPIStackInstHook {
	h := &CPIStackInstHook{
		timeTeller: timeTeller,
		cu:         cu,

		state:        "idle",
		greatestTask: "idle",

		inflightWfs:           make(map[string]tracing.Task),
		timeStack:             make(map[string]float64),
		inFlightInstBreakdown: map[string]uint64{},

		instChangeBreakdown: map[string]float64{
			"case0": 0,
			"case1": 0,
			"case2": 0,
			"case3": 0,
			"case4": 0,
			"case5": 0,
			"case6": 0,
		},
		taskHierarchy: map[string]uint64{
			"idle":    0,
			"fetch":   1,
			"Special": 2,
			"VMem":    3,
			"LDS":     4,
			"Scalar":  5,
			"VALU":    6,
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

	// fmt.Printf("%s, total_time, %.10f\n",
	// 	h.cu.Name(), h.lastWFEnd-h.firstWFStart)

	// fmt.Printf("%s, total_inst_count, %d\n",
	// 	h.cu.Name(), h.allInstCount)
	// fmt.Printf("%s, simd_inst_count, %d\n",
	// 	h.cu.Name(), h.simdInstCount)

	// for state, time := range h.timeStack {
	// 	fmt.Printf("%s, %s_time, %.10f\n",
	// 		h.cu.Name(), state, time)
	// }
}

// Func records issued instructions.
func (h *CPIStackInstHook) Func(ctx sim.HookCtx) {
	switch ctx.Pos {
	case tracing.HookPosTaskStart:
		task := ctx.Item.(tracing.Task)
		h.handleTaskStart(task)
	case tracing.HookPosTaskEnd:
		task := ctx.Item.(tracing.Task)
		h.handleTaskEnd(task)
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
	defer func() { h.lastTaskTime = float64(h.timeTeller.CurrentTime()) }()

	// fmt.Printf("%.10f, %s, %s-%s\n",
	// h.timeTeller.CurrentTime(), h.cu.Name(), task.Kind, task.What)

	h.updateInFlightInst(true, task)

	switch task.Kind {
	case "wavefront":
		h.inflightWfs[task.ID] = task
		if !h.firstWFStarted {
			h.firstWFStarted = true
			h.firstWFStart = float64(h.timeTeller.CurrentTime())
		}
	case "inst":
		h.handleInstStart(task)
	case "fetch":
		h.handleFetchStart(task)
	}
}

func (h *CPIStackInstHook) handleInstStart(task tracing.Task) {
	h.allInstCount++

	if task.What == "VALU" {
		h.simdInstCount++
	}
}

func (h *CPIStackInstHook) handleFetchStart(task tracing.Task) {
	switch h.state {
	case "idle":
		h.state = "fetch"
		h.addStackTime("idle", h.timeDiff())
	}
}

func (h *CPIStackInstHook) timeDiff() float64 {
	return float64(h.timeTeller.CurrentTime()) - h.lastTaskTime
}

func (h *CPIStackInstHook) addStackTime(state string, time float64) {
	_, ok := h.timeStack[state]
	if !ok {
		h.timeStack[state] = 0
	}

	h.timeStack[state] += time
}

func (h *CPIStackInstHook) updateInFlightInst(beginning bool, task tracing.Task) {
	if task.What != "*mem.ReadReq" && task.What != "*mem.WriteReq" &&
		task.What != "*protocol.MapWGReq" && task.What != "wavefront" {
		if beginning {
			h.inFlightInstBreakdown[task.What]++
			h.totalInFlightInst++
		} else {
			h.inFlightInstBreakdown[task.What]--
			h.totalInFlightInst--
		}
	}
}

func (h *CPIStackInstHook) handleTaskEnd(task tracing.Task) {
	_, ok := h.inflightWfs[task.ID]

	h.updateInFlightInst(false, task)

	if ok {
		delete(h.inflightWfs, task.ID)
		h.lastWFEnd = float64(h.timeTeller.CurrentTime())
	}
}
