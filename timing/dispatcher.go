package timing

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// KernelDispatchStatus keeps the state of the dispatching process
type KernelDispatchStatus struct {
	Packet         *kernels.HsaKernelDispatchPacket
	Grid           *kernels.Grid
	WGs            []*kernels.WorkGroup
	DispatchingWfs []*kernels.Wavefront
	DispatchingCU  core.Component
	CUBusy         map[core.Component]bool
}

// NewKernelDispatchStatus returns a newly created KernelDispatchStatus
func NewKernelDispatchStatus() *KernelDispatchStatus {
	s := new(KernelDispatchStatus)
	s.WGs = make([]*kernels.WorkGroup, 0)
	s.DispatchingWfs = make([]*kernels.Wavefront, 0)
	s.CUBusy = make(map[core.Component]bool)
	return s
}

// MapWGReq is a request that is send by the Dispatcher to a ComputeUnit to
// ask the ComputeUnit to reserve resources for the work-group
type MapWGReq struct {
	*core.ReqBase

	WG *kernels.WorkGroup
	Ok bool
}

// NewMapWGReq returns a newly created MapWGReq
func NewMapWGReq() *MapWGReq {
	r := new(MapWGReq)
	r.ReqBase = core.NewReqBase()
	return r
}

// A KernelDispatchEvent is a event to continue the kernel dispatch process
type KernelDispatchEvent struct {
	*core.BasicEvent
	Status *KernelDispatchStatus
}

// NewKernelDispatchEvent returne a newly created KernelDispatchEvent
func NewKernelDispatchEvent() *KernelDispatchEvent {
	e := new(KernelDispatchEvent)
	e.BasicEvent = core.NewBasicEvent()
	return e
}

// A Dispatcher is a component that can dispatch workgroups and wavefronts
// to ComputeUnits.
type Dispatcher struct {
	*core.BasicComponent

	CUs  []core.Component
	Freq core.Freq

	engine      core.Engine
	gridBuilder kernels.GridBuilder
}

// NewDispatcher creates a new dispatcher
func NewDispatcher(
	name string,
	engine core.Engine,
	gridBuilder kernels.GridBuilder,
) *Dispatcher {
	d := new(Dispatcher)
	d.BasicComponent = core.NewBasicComponent(name)
	d.CUs = make([]core.Component, 0)
	d.gridBuilder = gridBuilder
	d.engine = engine
	return d
}

// Recv starts processing incomming requests
//
// Dispatcher receives
//     KernelDispatchReq ---- Request the dispatcher to dispatch the requests
//     to the compute units
func (d *Dispatcher) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *kernels.LaunchKernelReq:
		return d.processKernelDispatchReq(req)
	default:
		log.Panicf("Unable to process request %s", reflect.TypeOf(req))
	}
	return nil
}

func (d *Dispatcher) processKernelDispatchReq(
	req *kernels.LaunchKernelReq,
) *core.Error {
	evt := NewKernelDispatchEvent()
	status := NewKernelDispatchStatus()
	evt.Status = status

	status.Packet = req.Packet

	evt.SetTime(d.Freq.NextTick(req.RecvTime()))
	evt.SetHandler(d)

	d.engine.Schedule(evt)

	return nil
}

// Handle performe actions when an event is triggered
//
// Dispatcher processes
//     KernalDispatchEvent ---- continues the kernel dispatching process.
func (d *Dispatcher) Handle(evt core.Event) error {
	return nil
}
