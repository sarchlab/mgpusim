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

	WG *kernels.WorkGroup
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

// DispatcherState defines the current state of the dispatcher
type DispatcherState int

const (
	DispatcherIdle DispatcherState = iota
	DispatcherToMapWG
	DispatcherWaitMapWGACK
	DispatcherToDispatchWF
	DispatcherWaitDispatchWFACK
)

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

	cus    []core.Component
	cuBusy map[core.Component]bool

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
	state           DispatcherState
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

	d.cus = make([]core.Component, 0)
	d.cuBusy = make(map[core.Component]bool, 0)
	d.dispatchingWGs = make([]*kernels.WorkGroup, 0)
	d.completedWGs = make([]*kernels.WorkGroup, 0)
	d.dispatchingWfs = make([]*kernels.Wavefront, 0)

	d.AddPort("ToCUs")
	d.AddPort("ToCommandProcessor")

	d.state = DispatcherIdle

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
	d.InvokeHook(req, d, core.OnRecvReq, nil)
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

	if ok {
		d.initKernelDispatching(req)
		d.scheduleMapWG(d.Freq.NextTick(req.RecvTime()))
	} else {
		d.replyLaunchKernelReq(false, req, req.Time())
	}

	return nil
}

func (d *Dispatcher) replyLaunchKernelReq(
	ok bool,
	req *kernels.LaunchKernelReq,
	now core.VTimeInSec,
) *core.Error {
	req.OK = ok
	req.SwapSrcAndDst()
	req.SetSendTime(req.RecvTime())
	return d.GetConnection("ToCommandProcessor").Send(req)
}

// handleMapWGEvent initiates work-group mapping
func (d *Dispatcher) handleMapWGEvent(evt *MapWGEvent) error {

	if len(d.dispatchingWGs) == 0 {
		d.state = DispatcherIdle
		return nil
	}

	cuID, hasAvailableCU := d.nextAvailableCU()
	if !hasAvailableCU {
		d.state = DispatcherIdle
		return nil
	}

	CU := d.cus[cuID]
	req := NewMapWGReq(d, CU, evt.Time(), d.dispatchingWGs[0])
	d.state = DispatcherWaitMapWGACK
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
	evt := NewMapWGEvent(time, d)
	d.engine.Schedule(evt)
}

// handleMapWGReq deals with the respond of the MapWGReq from a compute unit.
func (d *Dispatcher) handleMapWGReq(req *MapWGReq) error {
	if !req.Ok {
		d.state = DispatcherToMapWG
		d.cuBusy[d.cus[d.dispatchingCUID]] = true
		d.scheduleMapWG(req.RecvTime())
		return nil
	}

	wg := d.dispatchingWGs[0]
	d.dispatchingWGs = d.dispatchingWGs[1:]
	d.dispatchingWfs = append(d.dispatchingWfs, wg.Wavefronts...)
	d.state = DispatcherToDispatchWF
	d.scheduleDispatchWfEvent(d.Freq.NextTick(req.RecvTime()))

	return nil
}

func (d *Dispatcher) scheduleDispatchWfEvent(time core.VTimeInSec) {
	evt := NewDispatchWfEvent(time, d)
	d.engine.Schedule(evt)
}

func (d *Dispatcher) handleDispatchWfEvent(evt *DispatchWfEvent) error {
	wf := d.dispatchingWfs[0]
	cu := d.cus[d.dispatchingCUID]

	req := NewDispatchWfReq(d, cu, evt.Time(), wf)
	d.state = DispatcherWaitDispatchWFACK
	err := d.GetConnection("ToCUs").Send(req)
	if err != nil {
		d.scheduleDispatchWfEvent(err.EarliestRetry)
	}

	return nil
}

func (d *Dispatcher) handleDispatchWfReq(req *DispatchWfReq) error {
	d.dispatchingWfs = d.dispatchingWfs[1:]

	if len(d.dispatchingWfs) == 0 {
		d.state = DispatcherToMapWG
		d.scheduleMapWG(d.Freq.NextTick(req.Time()))
		return nil
	}

	d.state = DispatcherToDispatchWF
	d.scheduleDispatchWfEvent(d.Freq.NextTick(req.Time()))

	return nil
}

func (d *Dispatcher) handleWGFinishMesg(mesg *WGFinishMesg) error {
	d.completedWGs = append(d.completedWGs, mesg.WG)
	d.cuBusy[mesg.Src()] = false
	if len(d.dispatchingGrid.WorkGroups) == len(d.completedWGs) {
		d.replyKernelFinish(mesg.Time())
		return nil
	}

	if d.state == DispatcherIdle {
		d.state = DispatcherToMapWG
		d.scheduleMapWG(d.Freq.NextTick(mesg.Time()))
	}
	return nil
}

func (d *Dispatcher) replyKernelFinish(now core.VTimeInSec) {

	log.Printf("Kernel completed at %.12f\n", now)

	req := d.dispatchingReq
	req.SwapSrcAndDst()
	req.SetSendTime(now)
	d.GetConnection("ToCommandProcessor").Send(req)

	d.dispatchingReq = nil
}

func (d *Dispatcher) RegisterCU(cu core.Component) {
	d.cus = append(d.cus, cu)
	d.cuBusy[cu] = false
}

func (d *Dispatcher) nextAvailableCU() (int, bool) {
	count := len(d.cuBusy)
	cuID := d.dispatchingCUID
	for i := 0; i < count; i++ {
		cuID++
		if cuID >= len(d.cuBusy) {
			cuID = 0
		}

		if !d.cuBusy[d.cus[cuID]] {
			return cuID, true
		}
	}
	return -1, false
}
