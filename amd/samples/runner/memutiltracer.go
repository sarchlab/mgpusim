package runner

import (
	"sync"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// memUtilTracer tracks memory utilization metrics.
type memUtilTracer struct {
	sync.Mutex
	sim.TimeTeller

	inflightTasks map[string]tracing.Task

	// Basic counters
	readCount  uint64
	writeCount uint64
	readBytes  uint64
	writeBytes uint64

	// Timing information
	startTime sim.VTimeInSec
	endTime   sim.VTimeInSec

	// Outstanding requests tracking
	maxOutstanding     int
	currentOutstanding int
	totalOutstanding   uint64
	outstandingSamples uint64
}

func newMemUtilTracer(timeTeller sim.TimeTeller) *memUtilTracer {
	return &memUtilTracer{
		TimeTeller:    timeTeller,
		inflightTasks: make(map[string]tracing.Task),
		startTime:     sim.VTimeInSec(-1),
	}
}

// StartTask records the task start time
func (t *memUtilTracer) StartTask(task tracing.Task) {
	t.Lock()
	defer t.Unlock()

	task.StartTime = t.TimeTeller.CurrentTime()

	// Initialize start time on first task
	if t.startTime < 0 {
		t.startTime = sim.VTimeInSec(task.StartTime)
	}

	t.inflightTasks[task.ID] = task

	// Track outstanding requests
	t.currentOutstanding++
	if t.currentOutstanding > t.maxOutstanding {
		t.maxOutstanding = t.currentOutstanding
	}
	t.totalOutstanding += uint64(t.currentOutstanding)
	t.outstandingSamples++
}

// StepTask does nothing
func (t *memUtilTracer) StepTask(task tracing.Task) {
	// Do nothing
}

// AddMilestone does nothing
func (t *memUtilTracer) AddMilestone(milestone tracing.Milestone) {
	// Do nothing
}

// EndTask records the end of the task
func (t *memUtilTracer) EndTask(task tracing.Task) {
	t.Lock()
	defer t.Unlock()

	originalTask, ok := t.inflightTasks[task.ID]
	if !ok {
		return
	}

	task.EndTime = t.TimeTeller.CurrentTime()
	t.endTime = sim.VTimeInSec(task.EndTime)

	// Update counters based on request type
	switch originalTask.What {
	case "*mem.ReadReq":
		t.readCount++
		t.readBytes += originalTask.Detail.(*mem.ReadReq).AccessByteSize
	case "*mem.WriteReq":
		t.writeCount++
		t.writeBytes += uint64(len(originalTask.Detail.(*mem.WriteReq).Data))
	}

	// Track outstanding requests
	t.currentOutstanding--
	t.totalOutstanding += uint64(t.currentOutstanding)
	t.outstandingSamples++

	delete(t.inflightTasks, task.ID)
}

// GetReadBandwidth returns the read bandwidth in bytes/second
func (t *memUtilTracer) GetReadBandwidth() float64 {
	t.Lock()
	defer t.Unlock()

	duration := float64(t.endTime - t.startTime)
	if duration <= 0 {
		return 0
	}
	return float64(t.readBytes) / duration
}

// GetWriteBandwidth returns the write bandwidth in bytes/second
func (t *memUtilTracer) GetWriteBandwidth() float64 {
	t.Lock()
	defer t.Unlock()

	duration := float64(t.endTime - t.startTime)
	if duration <= 0 {
		return 0
	}
	return float64(t.writeBytes) / duration
}

// GetTotalBandwidth returns the total bandwidth in bytes/second
func (t *memUtilTracer) GetTotalBandwidth() float64 {
	return t.GetReadBandwidth() + t.GetWriteBandwidth()
}

// GetAverageOutstanding returns the average number of outstanding requests
func (t *memUtilTracer) GetAverageOutstanding() float64 {
	t.Lock()
	defer t.Unlock()

	if t.outstandingSamples == 0 {
		return 0
	}
	return float64(t.totalOutstanding) / float64(t.outstandingSamples)
}

// GetMaxOutstanding returns the maximum number of outstanding requests
func (t *memUtilTracer) GetMaxOutstanding() int {
	t.Lock()
	defer t.Unlock()

	return t.maxOutstanding
}

// GetReadCount returns the total number of read requests
func (t *memUtilTracer) GetReadCount() uint64 {
	t.Lock()
	defer t.Unlock()

	return t.readCount
}

// GetWriteCount returns the total number of write requests
func (t *memUtilTracer) GetWriteCount() uint64 {
	t.Lock()
	defer t.Unlock()

	return t.writeCount
}

// GetReadBytes returns the total number of bytes read
func (t *memUtilTracer) GetReadBytes() uint64 {
	t.Lock()
	defer t.Unlock()

	return t.readBytes
}

// GetWriteBytes returns the total number of bytes written
func (t *memUtilTracer) GetWriteBytes() uint64 {
	t.Lock()
	defer t.Unlock()

	return t.writeBytes
}
