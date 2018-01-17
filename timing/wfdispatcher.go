package timing

import (
	"gitlab.com/yaotsu/gcn3"
)

// A WfDispatcher initialize wavefronts
type WfDispatcher interface {
	DispatchWf(wf *Wavefront, req *gcn3.DispatchWfReq)
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
func (d *WfDispatcherImpl) DispatchWf(wf *Wavefront, req *gcn3.DispatchWfReq) {
	evt := NewWfDispatchCompletionEvent(
		d.cu.Freq.NCyclesLater(d.Latency, req.RecvTime()),
		d.cu, wf)
	evt.DispatchWfReq = req

	d.cu.engine.Schedule(evt)
	return
}
