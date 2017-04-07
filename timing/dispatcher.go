package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

type kernelDispatchStatus struct {
	packet         *kernels.HsaKernelDispatchPacket
	grid           *kernels.Grid
	wgs            []*kernels.WorkGroup
	dispatchingWfs []*kernels.Wavefront
	dispatchingCU  core.Component
	cuBusy         map[core.Component]bool
}

// MapWGReq is a request that is send by the Dispatcher to a ComputeUnit to
// ask the ComputeUnit to reserve resources for the work-group
type MapWGReq struct {
	*core.ReqBase

	WG *kernels.WorkGroup
	Ok bool
}

// A Dispatcher is a component that can dispatch workgroups and wavefronts
// to ComputeUnits.
type Dispatcher struct {
	*core.BasicComponent

	CUs []core.Component

	gridBuilder kernels.GridBuilder
}

// NewDispatcher creates a new dispatcher
func NewDispatcher(name string, gridBuilder kernels.GridBuilder) *Dispatcher {
	d := new(Dispatcher)
	d.BasicComponent = core.NewBasicComponent(name)
	d.CUs = make([]core.Component, 0)
	d.gridBuilder = gridBuilder
	return d
}

// Receive starts processing incomming requests
func (d *Dispatcher) Receive(req core.Req) *core.Error {
	return nil
}
