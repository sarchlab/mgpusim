package runner

import (
	"github.com/sarchlab/akita/v3/tracing"
)

// WavefrontCountTracer can trace the number of active wavefronts in a WavefrontPool.
type WavefrontCountTracer struct {
	count       uint64
	inflightWFs map[string]tracing.Task
}

// NewWavefrontCountTracer creates a tracer that can count the number of active wavefronts.
func NewWavefrontCountTracer() *WavefrontCountTracer {
	t := &WavefrontCountTracer{
		inflightWFs: map[string]tracing.Task{},
	}
	return t
}

func (t *WavefrontCountTracer) StartTask(task tracing.Task) {
	if task.Kind != "wavefront" {
		return
	}

	t.inflightWFs[task.ID] = task
	t.count++
}

func (t *WavefrontCountTracer) StepTask(task tracing.Task) {
	// Do nothing
}

func (t *WavefrontCountTracer) EndTask(task tracing.Task) {
	_, found := t.inflightWFs[task.ID]
	if !found {
		return
	}

	delete(t.inflightWFs, task.ID)
	t.count--
}

func (t *WavefrontCountTracer) GetCount() uint64 {
	return t.count
}
