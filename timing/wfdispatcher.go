package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
)

// WfDispatchCompletionEvent marks the completion of a wavefront dispatching
type WfDispatchCompletionEvent struct {
	*core.EventBase

	ManagedWf *Wavefront
}

// NewWfDispatchCompletionEvent creates a new WfDispatchCompletionEvent
func NewWfDispatchCompletionEvent(
	t core.VTimeInSec,
	handler core.Handler,
	Wf *Wavefront,
) *WfDispatchCompletionEvent {
	evt := new(WfDispatchCompletionEvent)
	evt.EventBase = core.NewEventBase(t, handler)
	evt.ManagedWf = Wf
	return evt
}

// A WfDispatcher initialize wavefronts
type WfDispatcher interface {
	DispatchWf(req *gcn3.DispatchWfReq)
}

// A WfDispatcherImpl will register the wavefront in wavefront pool and
// initialize all the registers
type WfDispatcherImpl struct {
	cu *ComputeUnit

	Latency int
}

// NewWfDispatcher creates a default WfDispatcher
func NewWfDispatcher(cu *ComputeUnit) *WfDispatcherImpl {
	d := new(WfDispatcherImpl)
	d.cu = cu
	d.Latency = 0
	return d
}

// DispatchWf starts or continues a wavefront dispatching process.
func (d *WfDispatcherImpl) DispatchWf(req *gcn3.DispatchWfReq) {
	wf := new(Wavefront)
	wf.Wavefront = req.Wf

	evt := NewWfDispatchCompletionEvent(
		d.cu.Freq.NCyclesLater(d.Latency, req.RecvTime()),
		d.cu, wf)

	d.cu.engine.Schedule(evt)
	return
}
