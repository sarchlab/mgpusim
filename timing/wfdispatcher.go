package timing

import (
	"gitlab.com/yaotsu/gcn3"
)

// A WfDispatcher initialize wavefronts
type WfDispatcher interface {
	DispatchWf(req *gcn3.DispatchWfReq) (bool, *Wavefront)
}

// A WfDispatcherImpl will register the wavefront in wavefront pool and
// initialize all the registers
type WfDispatcherImpl struct {
	cu *ComputeUnit
}

// DispatchWf starts or continues a wavefront dispatching process.
func (d *WfDispatcherImpl) DispatchWf(req *gcn3.DispatchWfReq) (bool, *Wavefront) {
	return true, nil
}
