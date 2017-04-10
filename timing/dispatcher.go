package timing

import (
	"log"
	"reflect"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// KernelDispatchStatus keeps the state of the dispatching process
type KernelDispatchStatus struct {
	Packet          *kernels.HsaKernelDispatchPacket
	Grid            *kernels.Grid
	WGs             []*kernels.WorkGroup
	DispatchingWfs  []*kernels.Wavefront
	DispatchingCUID int
	Mapped          bool

	CUBusy []bool
}

// NewKernelDispatchStatus returns a newly created KernelDispatchStatus
func NewKernelDispatchStatus() *KernelDispatchStatus {
	s := new(KernelDispatchStatus)
	s.WGs = make([]*kernels.WorkGroup, 0)
	s.DispatchingWfs = make([]*kernels.Wavefront, 0)
	s.CUBusy = make([]bool, 0)
	return s
}

// MapWGReq is a request that is send by the Dispatcher to a ComputeUnit to
// ask the ComputeUnit to reserve resources for the work-group
type MapWGReq struct {
	*core.ReqBase

	WG           *kernels.WorkGroup
	KernelStatus *KernelDispatchStatus
	Ok           bool
}

// NewMapWGReq returns a newly created MapWGReq
func NewMapWGReq(
	src, dst core.Component,
	time core.VTimeInSec,
	wg *kernels.WorkGroup,
	status *KernelDispatchStatus,
) *MapWGReq {
	r := new(MapWGReq)
	r.ReqBase = core.NewReqBase()
	r.SetSrc(src)
	r.SetDst(dst)
	r.SetSendTime(time)
	r.WG = wg
	r.KernelStatus = status
	return r
}

// A DispatchWfReq is the request to dispatch a wavefron to the compute unit
type DispatchWfReq struct {
	*core.ReqBase
	Wf *kernels.Wavefront
}

// NewDispatchWfReq creates a DispatchWfReq
func NewDispatchWfReq(
	src, dst core.Component,
	time core.VTimeInSec,
	wf *kernels.Wavefront,
) *DispatchWfReq {
	r := new(DispatchWfReq)
	r.ReqBase = core.NewReqBase()
	r.SetSrc(src)
	r.SetDst(dst)
	r.SetSendTime(time)
	r.Wf = wf
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
//
//     <=> ToCUs The connection that is connecting the dispatcher and the
//         compute units
type Dispatcher struct {
	*core.BasicComponent
	sync.Mutex

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

	d.AddPort("ToCUs")

	return d
}

// Recv starts processing incomming requests
//
// Dispatcher receives
//
//     KernelDispatchReq ---- Request the dispatcher to dispatch the requests
//                            to the compute units
//
//     MapWGReq ---- The request return from the compute unit tells if the
//                   compute unit is able to run the work-group
//
func (d *Dispatcher) Recv(req core.Req) *core.Error {
	d.Lock()
	defer d.Unlock()

	switch req := req.(type) {
	case *kernels.LaunchKernelReq:
		return d.processLaunchKernelReq(req)
	case *MapWGReq:
		return d.processMapWGReq(req)
	default:
		log.Panicf("Unable to process request %s", reflect.TypeOf(req))
	}
	return nil
}

func (d *Dispatcher) processLaunchKernelReq(
	req *kernels.LaunchKernelReq,
) *core.Error {
	evt := NewKernelDispatchEvent()
	status := NewKernelDispatchStatus()
	evt.Status = status

	status.Packet = req.Packet
	status.Grid = d.gridBuilder.Build(req)
	status.WGs = append(status.WGs, status.Grid.WorkGroups...)
	status.DispatchingCUID = -1

	for _ = range d.CUs {
		status.CUBusy = append(status.CUBusy, false)
	}

	evt.SetTime(d.Freq.NextTick(req.RecvTime()))
	evt.SetHandler(d)

	d.engine.Schedule(evt)

	return nil
}

func (d *Dispatcher) processMapWGReq(req *MapWGReq) *core.Error {
	status := req.KernelStatus

	if req.Ok {
		status.DispatchingWfs = append(status.DispatchingWfs,
			req.WG.Wavefronts...)
		status.Mapped = true
	} else {
		status.CUBusy[status.DispatchingCUID] = true
	}

	evt := NewKernelDispatchEvent()
	evt.SetTime(d.Freq.NextTick(req.RecvTime()))
	evt.SetHandler(d)
	evt.Status = status
	d.engine.Schedule(evt)

	return nil
}

// Handle performe actions when an event is triggered
//
// Dispatcher processes
//     KernalDispatchEvent ---- continues the kernel dispatching process.
//
func (d *Dispatcher) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *KernelDispatchEvent:
		d.handleKernalDispatchEvent(evt)
	default:
		log.Panicf("Unable to process evevt %+v", evt)
	}
	return nil
}

func (d *Dispatcher) handleKernalDispatchEvent(evt *KernelDispatchEvent) error {
	status := evt.Status
	if status.Mapped {
		d.dispatchWf(evt)
	} else {
		d.mapWG(evt)
	}

	return nil
}

func (d *Dispatcher) dispatchWf(evt *KernelDispatchEvent) {
	status := evt.Status
	req := NewDispatchWfReq(d, d.CUs[status.DispatchingCUID], evt.Time(),
		status.DispatchingWfs[0])

	err := d.GetConnection("ToCUs").Send(req)
	if err != nil && err.Recoverable {
		log.Panic(err)
	} else if err != nil {
		evt.SetTime(d.Freq.NoEarlierThan(err.EarliestRetry))
		d.engine.Schedule(evt)
	} else {
		status.DispatchingWfs = status.DispatchingWfs[1:]
	}
}

func (d *Dispatcher) mapWG(evt *KernelDispatchEvent) {
	status := evt.Status
	if !d.isAllCUsBusy(status) {
		cuID := d.nextAvailableCU(status)
		cu := d.CUs[cuID]
		wg := status.WGs[0]
		req := NewMapWGReq(d, cu, evt.Time(), wg, status)

		d.GetConnection("ToCUs").Send(req)
	}

}

func (d *Dispatcher) isAllCUsBusy(status *KernelDispatchStatus) bool {
	for _, busy := range status.CUBusy {
		if !busy {
			return false
		}
	}
	return true
}

func (d *Dispatcher) nextAvailableCU(status *KernelDispatchStatus) int {
	startingFrom := status.DispatchingCUID
	cuID := startingFrom
	for {
		cuID++
		if cuID >= len(status.CUBusy) {
			cuID = 0
		}
		if cuID == startingFrom {
			return -1 // Not found. Should call isAllCUBusy to check first.
		}

		if !status.CUBusy[cuID] {
			return cuID
		}
	}
}
