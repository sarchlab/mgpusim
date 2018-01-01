package gcn3

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// MapWGReq is a request that is send by the Dispatcher to a ComputeUnit to
// ask the ComputeUnit to reserve resources for the work-group
type MapWGReq struct {
	*core.ReqBase

	WG               *kernels.WorkGroup
	Ok               bool
	CUOutOfResources bool
}

// NewMapWGReq returns a newly created MapWGReq
func NewMapWGReq(
	src, dst core.Component,
	time core.VTimeInSec,
	wg *kernels.WorkGroup,
) *MapWGReq {
	r := new(MapWGReq)
	r.ReqBase = core.NewReqBase()
	r.SetSrc(src)
	r.SetDst(dst)
	r.SetSendTime(time)
	r.WG = wg
	return r
}

// A MapWGEvent is an event used by the dispatcher to map a work-group
type MapWGEvent struct {
	*core.EventBase
}

// NewMapWGEvent creates a new MapWGEvent
func NewMapWGEvent(t core.VTimeInSec, handler core.Handler) *MapWGEvent {
	e := new(MapWGEvent)
	e.EventBase = core.NewEventBase(t, handler)
	return e
}

// A DispatchWfReq is the request to dispatch a wavefront to the compute unit
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

// A DispatchWfEvent is an event used by the dispatcher to dispatch a wavefront
type DispatchWfEvent struct {
	*core.EventBase
}

// NewDispatchWfEvent creates a new DispatchWfEvent
func NewDispatchWfEvent(
	t core.VTimeInSec,
	handler core.Handler,
) *DispatchWfEvent {
	e := new(DispatchWfEvent)
	e.EventBase = core.NewEventBase(t, handler)
	return e
}

// A WGFinishMesg is sent by a compute unit to notify about the completion of
// a work-group
type WGFinishMesg struct {
	*core.ReqBase

	WG   *kernels.WorkGroup
	CUID int
}

// NewWGFinishMesg creates and returns a newly created WGFinishMesg
func NewWGFinishMesg(
	src, dst core.Component,
	time core.VTimeInSec,
	wg *kernels.WorkGroup,
) *WGFinishMesg {
	m := new(WGFinishMesg)
	m.ReqBase = core.NewReqBase()

	m.SetSrc(src)
	m.SetDst(dst)
	m.SetSendTime(time)
	m.WG = wg

	return m
}

// A Dispatcher is a component that can dispatch work-groups and wavefronts
// to ComputeUnits.
//
//     <=> ToCUs The connection that is connecting the dispatcher and the
//         compute units
//
//     <=> ToCommandProcessor The connection that is connecting the dispatcher
//         with the command processor
//
type Dispatcher struct {
	*core.ComponentBase

	CUs    []core.Component
	CUBusy []bool

	engine      core.Engine
	gridBuilder kernels.GridBuilder
	Freq        util.Freq

	// The request that is being processed, one dispatcher can only dispatch one kernel at a time.
	dispatchingReq  *kernels.LaunchKernelReq
	dispatchingGrid *kernels.Grid
	dispatchingWGs  []*kernels.WorkGroup
	completedWGs    []*kernels.WorkGroup
	dispatchingWfs  []*kernels.Wavefront
	dispatchingCUID int

	// If the dispatcher has pending MapWGEvent or DispatchWfEvent, no other
	// events should be scheduled.
	hasPendingEvent bool
}

// NewDispatcher creates a new dispatcher
func NewDispatcher(
	name string,
	engine core.Engine,
	gridBuilder kernels.GridBuilder,
) *Dispatcher {
	d := new(Dispatcher)
	d.ComponentBase = core.NewComponentBase(name)

	d.gridBuilder = gridBuilder
	d.engine = engine

	d.CUs = make([]core.Component, 0)
	d.CUBusy = make([]bool, 0)
	d.dispatchingWGs = make([]*kernels.WorkGroup, 0)
	d.completedWGs = make([]*kernels.WorkGroup, 0)
	d.dispatchingWfs = make([]*kernels.Wavefront, 0)

	d.AddPort("ToCUs")
	d.AddPort("ToCommandProcessor")

	return d
}

// Recv starts processing incoming requests
//
// The protocol that is defined by the dispatcher is as follows:
//
// When the dispatcher receives a LaunchKernelReq request from the command
// processor, the kernel launching process is started. One dispatcher can only
// process one kernel at a time. So if the dispatcher is busy when the
// LaunchKernel is received, an NACK will be replied to the command processor.
//
// During the kernel dispatching process, the dispatcher will first check if
// the next compute unit can map a workgroup or not by sending a MapWGReq.
// The selection of the compute unit is in a round-robin fashion. If the
// compute unit can map a work-group, the dispatcher will dispatch wavefronts
// onto the compute unit by sending DispatchWfReq. The dispatcher will wait
// for the compute unit to return completion message for the DispatchWfReq
// before dispatching the next wavefront.
//
// Dispatcher receives
//
//     KernelDispatchReq ---- Request the dispatcher to dispatch the a kernel
//                            to the compute units
//
//     MapWGReq ---- The request return from the compute unit tells if the
//                   compute unit is able to run the work-group
//
//     WGFinishMesg ---- The CU send this message to the dispatcher to notify
//                       the completion of a workgroup
//
func (d *Dispatcher) Recv(req core.Req) *core.Error {
	util.ProcessReqAsEvent(req, d.engine, d.Freq)
	return nil
}

// Handle perform actions when an event is triggered
func (d *Dispatcher) Handle(evt core.Event) error {
	d.Lock()
	defer d.Unlock()

	d.InvokeHook(evt, d, core.BeforeEvent, nil)
	defer d.InvokeHook(evt, d, core.AfterEvent, nil)

	switch evt := evt.(type) {
	case *kernels.LaunchKernelReq:
		return d.handleLaunchKernelReq(evt)
	case *MapWGEvent:
		return d.handleMapWGEvent(evt)
	case *MapWGReq:
		return d.handleMapWGReq(evt)
	case *DispatchWfEvent:
		return d.handleDispatchWfEvent(evt)
	case *DispatchWfReq:
		return d.handleDispatchWfReq(evt)
	case *WGFinishMesg:
		return d.handleWGFinishMesg(evt)

	default:
		log.Panicf("Unable to process evevt of type %s", reflect.TypeOf(evt))
	}

	return nil
}

func (d *Dispatcher) handleLaunchKernelReq(
	req *kernels.LaunchKernelReq,
) error {

	var ok bool
	if d.dispatchingReq != nil {
		ok = false
	} else {
		ok = true
	}

	d.replyLaunchKernelReq(ok, req)

	if ok {
		d.initKernelDispatching(req)
		d.scheduleMapWG(d.Freq.NextTick(req.RecvTime()))
	}

	return nil
}

func (d *Dispatcher) replyLaunchKernelReq(ok bool, req *kernels.LaunchKernelReq) {
	req.OK = ok
	req.SwapSrcAndDst()
	req.SetSendTime(req.RecvTime())
	d.GetConnection("ToCommandProcessor").Send(req)
}

// handleMapWGEvent initiates work-group mapping
func (d *Dispatcher) handleMapWGEvent(evt *MapWGEvent) error {
	d.hasPendingEvent = false

	if len(d.dispatchingWGs) == 0 {
		return nil
	}

	cuID, hasAvailableCU := d.nextAvailableCU()
	if !hasAvailableCU {
		return nil
	}

	CU := d.CUs[cuID]
	req := NewMapWGReq(d, CU, evt.Time(), d.dispatchingWGs[0])
	err := d.GetConnection("ToCUs").Send(req)
	if err != nil {
		d.scheduleMapWG(err.EarliestRetry)
		return nil
	}

	d.dispatchingCUID = cuID

	return nil
}

func (d *Dispatcher) initKernelDispatching(req *kernels.LaunchKernelReq) {
	d.dispatchingReq = req
	d.dispatchingGrid = d.gridBuilder.Build(req)
	d.dispatchingWGs = append(d.dispatchingWGs, d.dispatchingGrid.WorkGroups...)

	d.dispatchingCUID = -1
}

func (d *Dispatcher) scheduleMapWG(time core.VTimeInSec) {
	if !d.hasPendingEvent {
		evt := NewMapWGEvent(time, d)
		d.engine.Schedule(evt)
	}
}

// handleMapWGReq deals with the respond of the MapWGReq from a compute unit.
func (d *Dispatcher) handleMapWGReq(req *MapWGReq) error {
	if !req.Ok {
		d.CUBusy[d.dispatchingCUID] = true
		d.scheduleMapWG(req.RecvTime())
		return nil
	}

	wg := d.dispatchingWGs[0]
	d.dispatchingWGs = d.dispatchingWGs[1:]
	d.dispatchingWfs = append(d.dispatchingWfs, wg.Wavefronts...)
	d.scheduleDispatchWfEvent(d.Freq.NextTick(req.RecvTime()))

	return nil
}

func (d *Dispatcher) scheduleDispatchWfEvent(time core.VTimeInSec) {
	if !d.hasPendingEvent {
		evt := NewDispatchWfEvent(time, d)
		d.engine.Schedule(evt)
	}
}

func (d *Dispatcher) handleDispatchWfEvent(evt *DispatchWfEvent) error {
	d.hasPendingEvent = false
	wf := d.dispatchingWfs[0]
	cu := d.CUs[d.dispatchingCUID]

	req := NewDispatchWfReq(d, cu, evt.Time(), wf)
	err := d.GetConnection("ToCUs").Send(req)
	if err != nil {
		d.scheduleDispatchWfEvent(err.EarliestRetry)
	}

	return nil
}

func (d *Dispatcher) handleDispatchWfReq(req *DispatchWfReq) error {
	if len(d.dispatchingWfs) <= 1 {
		d.scheduleMapWG(d.Freq.NextTick(req.RecvTime()))
		return nil
	}

	d.dispatchingWfs = d.dispatchingWfs[1:]
	wf := d.dispatchingWfs[0]

	nextReq := NewDispatchWfReq(d, d.CUs[d.dispatchingCUID], req.Time(), wf)
	err := d.GetConnection("ToCUs").Send(nextReq)
	if err != nil && !err.Recoverable {
		log.Panic(err)
	} else if err != nil {
		d.scheduleDispatchWfEvent(err.EarliestRetry)
	}

	return nil
}

func (d *Dispatcher) handleWGFinishMesg(mesg *WGFinishMesg) error {
	d.completedWGs = append(d.completedWGs, mesg.WG)
	if len(d.dispatchingGrid.WorkGroups) == len(d.completedWGs) {
		d.replyKernelFinish(mesg.Time())
		return nil
	}

	d.scheduleMapWG(d.Freq.NextTick(mesg.Time()))
	return nil
}

func (d *Dispatcher) replyKernelFinish(now core.VTimeInSec) {
	req := d.dispatchingReq
	req.SwapSrcAndDst()
	req.SetSendTime(now)
	d.GetConnection("ToCommandProcessor").Send(req)

	d.dispatchingReq = nil
}

//
//func (d *Dispatcher) processWGFinishWGMesg(mesg *WGFinishMesg) *core.Error {
//	status := d.dispatchingKernel
//	status.CompletedWGs = append(status.CompletedWGs, mesg.WG)
//
//	if len(status.CompletedWGs) == len(status.Grid.WorkGroups) {
//		status.Req.SwapSrcAndDst()
//		d.GetConnection("ToCommandProcessor").Send(status.Req)
//	} else {
//		status.CUBusy[mesg.CUID] = false
//		d.tryScheduleTick(d.Freq.NextTick(d.Freq.NextTick(mesg.RecvTime())))
//	}
//
//	return nil
//}
//
//func (d *Dispatcher) handleTickEvent(evt *core.TickEvent) error {
//	status := d.dispatchingKernel
//	if status.Mapped {
//		d.dispatchWf(evt.Time())
//	} else {
//		d.mapWG(evt.Time())
//	}
//
//	return nil
//}
//
//func (d *Dispatcher) dispatchWf(now core.VTimeInSec) {
//	status := d.dispatchingKernel
//
//	// In case there is no wf to disaptch
//	if len(status.DispatchingWfs) == 0 {
//		status.Mapped = false
//		if len(status.WGs) > 0 {
//			d.scheduleTick(d.Freq.NextTick(now))
//		}
//		return
//	}
//
//	entryPoint := status.Grid.Packet.KernelObject +
//		status.Grid.CodeObject.KernelCodeEntryByteOffset
//
//	info := status.DispatchingWfs[0]
//	wf := info.Wavefront
//	req := NewDispatchWfReq(d, d.CUs[status.DispatchingCUID], now,
//		wf, info, entryPoint)
//	req.CodeObject = status.CodeObject
//	req.Packet = status.Packet
//
//	err := d.GetConnection("ToCUs").Send(req)
//	if err != nil && err.Recoverable {
//		log.Panic(err)
//	} else if err != nil {
//		d.scheduleTick(d.Freq.NoEarlierThan(err.EarliestRetry))
//	} else {
//		status.DispatchingWfs = status.DispatchingWfs[1:]
//
//		if len(status.DispatchingWfs) == 0 {
//			status.Mapped = false
//		}
//
//		if len(status.DispatchingWfs) > 0 || len(status.WGs) > 0 {
//			d.scheduleTick(d.Freq.NextTick(now))
//		}
//	}
//}
//
//func (d *Dispatcher) mapWG(now core.VTimeInSec) {
//	status := d.dispatchingKernel
//	if len(status.WGs) != 0 && !d.isAllCUsBusy(status) {
//		cuID := d.nextAvailableCU(status)
//		cu := d.CUs[cuID]
//		wg := status.WGs[0]
//		req := NewMapWGReq(d, cu, now, wg, status.CodeObject)
//		req.CUID = cuID
//
//		log.Printf("Trying to map wg to cu %d\n", cuID)
//		d.GetConnection("ToCUs").Send(req)
//	}
//
//	// Always pause the dispatching and wait for the reply to determine whether
//	// to continue
//	d.running = false
//}

func (d *Dispatcher) RegisterCU(cu core.Component) {
	d.CUs = append(d.CUs, cu)
	d.CUBusy = append(d.CUBusy, false)
}

func (d *Dispatcher) isAllCUsBusy() bool {
	for _, busy := range d.CUBusy {
		if !busy {
			return false
		}
	}
	return true
}

func (d *Dispatcher) nextAvailableCU() (int, bool) {
	count := len(d.CUBusy)
	cuID := d.dispatchingCUID
	for i := 0; i < count; i++ {
		cuID++
		if cuID >= len(d.CUBusy) {
			cuID = 0
		}

		if !d.CUBusy[cuID] {
			return cuID, true
		}
	}
	return -1, false
}
