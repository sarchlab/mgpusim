package cu

import (
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// secondsOf converts a simulated time in picoseconds to seconds, for the
// human-readable statistics this tracer reports.
func secondsOf(t timing.VTimeInPicoSec) float64 {
	return float64(t) * 1e-12
}

// InstTracer is a tracer that traces the time that VALU instructions take
// and VMem instructions take
type InstTracer struct {
	timeTeller            timing.TimeTeller
	SIMDCPIStackValues    SIMDCPIStack
	AllInstCPIStackValues AllInstCPIStack
	TimeManager           InstTimeManagement
	CountManager          InstCountManagement
	ScalarInstTracer      *tracing.BusyTimeTracer
	inflightInsts         map[uint64]tracing.TaskStart
}

// SIMDCPIStack holds the cpi stack values of each type of instruction when
// looking at the SIMD specifically
type SIMDCPIStack struct {
	SIMDCPI     float64
	LDSStack    float64
	BranchStack float64
	ScalarStack float64
	VMemStack   float64
	StackBase   float64
}

// AllInstCPIStack holds the cpi stack values of each type of instruction
// when considering each instruction in total
type AllInstCPIStack struct {
	AllInstCPI     float64
	SIMDStack      float64
	LDSStack       float64
	BranchStack    float64
	ScalarStack    float64
	VMemStack      float64
	TotalStackBase float64
}

// InstTimeManagement manages the time distribution in tracing. All times are
// in seconds.
type InstTimeManagement struct {
	TotalVMemTime    float64
	TotalBranchTime  float64
	TotalSpecialTime float64
	TotalLDSTime     float64
	TotalTime        float64
	TotalSIMDTime    float64
	TotalOtherTime   float64
	firstInstStart   float64
}

// InstCountManagement manages the instruction count in tracing
type InstCountManagement struct {
	TotalInstCount uint64
	SIMDInstCount  uint64
	OtherInstCount uint64
}

// NewInstTracer creates a new InstTracer
func NewInstTracer(timeTeller timing.TimeTeller) *InstTracer {
	return &InstTracer{
		timeTeller:            timeTeller,
		SIMDCPIStackValues:    *newSIMDCPIStack(),
		AllInstCPIStackValues: *newTotalCPIStack(),
		TimeManager:           *newInstTimeManager(),
		CountManager:          *newInstCountManager(),
		ScalarInstTracer:      tracing.NewBusyTimeTracer(nil),
		inflightInsts:         make(map[uint64]tracing.TaskStart),
	}
}

// newSIMDCPIStack creates a new SIMD specific CPI stack
func newSIMDCPIStack() *SIMDCPIStack {
	return &SIMDCPIStack{
		StackBase: 4,
	}
}

// newTotalCPIStack creates an overall instruction CPI stack
func newTotalCPIStack() *AllInstCPIStack {
	return &AllInstCPIStack{}
}

// newInstTimeManager creates a new time manager
func newInstTimeManager() *InstTimeManagement {
	return &InstTimeManagement{}
}

// newInstCountManager creates a new instruction count manager
func newInstCountManager() *InstCountManagement {
	return &InstCountManagement{}
}

// StartTask begins the tracing for the InstTracer
func (t *InstTracer) StartTask(task tracing.TaskStart) {
	if task.Kind != "inst" {
		return
	}

	if task.What == "Scalar" {
		t.ScalarInstTracer.StartTask(task)
	}

	if t.CountManager.TotalInstCount == 0 {
		t.TimeManager.firstInstStart = secondsOf(task.Time)
	}

	t.inflightInsts[task.ID] = task
}

// AddTaskTag does nothing for now
func (t *InstTracer) AddTaskTag(tag tracing.TaskTag) {
	// Do nothing
}

// AddMilestone does nothing for now
func (t *InstTracer) AddMilestone(milestone tracing.Milestone) {
	// Do nothing
}

// EndTask filters time into correct attribute and deletes the instruction
// from the tracer
func (t *InstTracer) EndTask(task tracing.TaskEnd) {
	orgTask, ok := t.inflightInsts[task.ID]

	if !ok {
		return
	}

	endTime := secondsOf(task.Time)
	timeDiff := endTime - secondsOf(orgTask.Time)

	t.calcTotalTime(endTime)

	if orgTask.What == "VALU" {
		t.TimeManager.TotalSIMDTime += timeDiff
		t.CountManager.SIMDInstCount++
	} else {
		t.CountManager.OtherInstCount++

		if orgTask.What == "VMem" {
			t.TimeManager.TotalVMemTime += timeDiff
		} else if orgTask.What == "Special" {
			t.TimeManager.TotalSpecialTime += timeDiff
		} else if orgTask.What == "Branch" {
			t.TimeManager.TotalBranchTime += timeDiff
		} else if orgTask.What == "Scalar" {
			t.ScalarInstTracer.EndTask(task)
		} else if orgTask.What == "LDS" {
			t.TimeManager.TotalLDSTime += timeDiff
		}
	}

	t.CountManager.TotalInstCount++

	t.calcTimeDistribution()

	delete(t.inflightInsts, task.ID)
}

// calcTotalTime calculates the total time for the instructions to execute
func (t *InstTracer) calcTotalTime(currentEndTime float64) {
	t.TimeManager.TotalTime = currentEndTime - t.TimeManager.firstInstStart
}

// calcTimeDistribution calculates the time distribution for the instructions
// set
func (t *InstTracer) calcTimeDistribution() {
	t.TimeManager.TotalOtherTime =
		t.TimeManager.TotalTime - t.TimeManager.TotalSIMDTime
}

// CalcSIMDCPIStack calculates the SIMD Specific CPI Stack
func (t *InstTracer) CalcSIMDCPIStack() {
	scalarBusyTime := secondsOf(t.ScalarInstTracer.BusyTime())

	t.SIMDCPIStackValues.SIMDCPI =
		(t.TimeManager.TotalTime * float64(1000000000)) /
			float64(t.CountManager.SIMDInstCount)
	t.SIMDCPIStackValues.BranchStack =
		(t.TimeManager.TotalBranchTime / t.TimeManager.TotalTime) *
			t.SIMDCPIStackValues.SIMDCPI
	t.SIMDCPIStackValues.ScalarStack =
		(scalarBusyTime / t.TimeManager.TotalTime) *
			t.SIMDCPIStackValues.SIMDCPI
	t.SIMDCPIStackValues.LDSStack =
		(t.TimeManager.TotalLDSTime / t.TimeManager.TotalTime) *
			t.SIMDCPIStackValues.SIMDCPI
}

// CalcTotalCPIStack calculates the CPI Stack for all instructions
func (t *InstTracer) CalcTotalCPIStack() {
	scalarBusyTime := secondsOf(t.ScalarInstTracer.BusyTime())

	t.AllInstCPIStackValues.AllInstCPI =
		(t.TimeManager.TotalTime * float64(1000000000)) /
			float64(t.CountManager.TotalInstCount)
	t.AllInstCPIStackValues.SIMDStack =
		(t.TimeManager.TotalSIMDTime / t.TimeManager.TotalTime) *
			t.AllInstCPIStackValues.AllInstCPI
	t.AllInstCPIStackValues.BranchStack =
		(t.TimeManager.TotalBranchTime / t.TimeManager.TotalTime) *
			t.AllInstCPIStackValues.AllInstCPI
	t.AllInstCPIStackValues.ScalarStack =
		(scalarBusyTime / t.TimeManager.TotalTime) *
			t.AllInstCPIStackValues.AllInstCPI
	t.AllInstCPIStackValues.LDSStack =
		(t.TimeManager.TotalLDSTime / t.TimeManager.TotalTime) *
			t.AllInstCPIStackValues.AllInstCPI
}
