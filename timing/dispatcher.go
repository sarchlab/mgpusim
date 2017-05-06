package timing

import (
	"log"
	"reflect"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// KernelDispatchStatus keeps the state of the dispatching process
type KernelDispatchStatus struct {
	Req             *kernels.LaunchKernelReq
	Packet          *kernels.HsaKernelDispatchPacket
	CodeObject      *insts.HsaCo
	Grid            *kernels.Grid
	WGs             []*kernels.WorkGroup
	CompletedWGs    []*kernels.WorkGroup
	DispatchingWfs  map[*kernels.Wavefront]*WfDispatchInfo
	DispatchingCUID int
	Mapped          bool
	CUBusy          []bool
}

// NewKernelDispatchStatus returns a newly created KernelDispatchStatus
func NewKernelDispatchStatus() *KernelDispatchStatus {
	s := new(KernelDispatchStatus)
	s.WGs = make([]*kernels.WorkGroup, 0)
	s.CompletedWGs = make([]*kernels.WorkGroup, 0)
	s.DispatchingWfs = make(map[*kernels.Wavefront]*WfDispatchInfo)
	s.CUBusy = make([]bool, 0)
	return s
}

// WfDispatchInfo stores the information about where the wf should dispatch to.
// When the dispatcher maps the workgroup, the compute unit should tell the
// dispatcher where to dispatch the wavefront.
type WfDispatchInfo struct {
	SIMDID     int
	VGPROffset int
	SGPROffset int
	LDSOffset  int
}

// MapWGReq is a request that is send by the Dispatcher to a ComputeUnit to
// ask the ComputeUnit to reserve resources for the work-group
type MapWGReq struct {
	*core.ReqBase

	WG            *kernels.WorkGroup
	KernelStatus  *KernelDispatchStatus
	Ok            bool
	WfDispatchMap map[*kernels.Wavefront]*WfDispatchInfo // Tells where a wf should fit in
	CUID          int
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

	r.WfDispatchMap = make(map[*kernels.Wavefront]*WfDispatchInfo)
	return r
}

// A DispatchWfReq is the request to dispatch a wavefron to the compute unit
type DispatchWfReq struct {
	*core.ReqBase
	Wf         *kernels.Wavefront
	CodeObject *insts.HsaCo
	Packet     *kernels.HsaKernelDispatchPacket
	Info       *WfDispatchInfo
	EntryPoint uint64
}

// NewDispatchWfReq creates a DispatchWfReq
func NewDispatchWfReq(
	src, dst core.Component,
	time core.VTimeInSec,
	wf *kernels.Wavefront,
	info *WfDispatchInfo,
	EntryPoint uint64,
) *DispatchWfReq {
	r := new(DispatchWfReq)
	r.ReqBase = core.NewReqBase()
	r.SetSrc(src)
	r.SetDst(dst)
	r.SetSendTime(time)
	r.Wf = wf
	r.Info = info
	r.EntryPoint = EntryPoint
	return r
}

// A WGFinishMesg is sent by a compute unit to noitify about the completion of
// a workgroup
type WGFinishMesg struct {
	*core.ReqBase

	WG     *kernels.WorkGroup
	Status *KernelDispatchStatus
}

// NewWGFinishMesg creates and returns a newly created WGFinishMesg
func NewWGFinishMesg(
	src, dst core.Component,
	time core.VTimeInSec,
	wg *kernels.WorkGroup,
	status *KernelDispatchStatus,
) *WGFinishMesg {
	m := new(WGFinishMesg)
	m.ReqBase = core.NewReqBase()

	m.SetSrc(src)
	m.SetDst(dst)
	m.SetSendTime(time)
	m.WG = wg
	m.Status = status

	return m
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
//
//     <=> ToCommandProcessor The connection that is connecting the dispatcher
//         with the command processor
//
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
	d.AddPort("ToCommandProcessor")

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
//     WGFinishMesg ---- The CU send this message to the dispatcher to notify
//                       the completion of a workgroup
//
func (d *Dispatcher) Recv(req core.Req) *core.Error {
	d.Lock()
	defer d.Unlock()

	switch req := req.(type) {
	case *kernels.LaunchKernelReq:
		return d.processLaunchKernelReq(req)
	case *MapWGReq:
		return d.processMapWGReq(req)
	case *WGFinishMesg:
		return d.processWGFinishWGMesg(req)
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

	status.Req = req
	status.Packet = req.Packet
	status.Grid = d.gridBuilder.Build(req)
	status.WGs = append(status.WGs, status.Grid.WorkGroups...)
	status.DispatchingCUID = -1
	status.CodeObject = req.HsaCo

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
		status.DispatchingWfs = req.WfDispatchMap
		status.DispatchingCUID = req.CUID
		status.Mapped = true
	} else {
		log.Printf("Marking cu %d as busy\n", req.CUID)
		status.CUBusy[req.CUID] = true
	}

	evt := NewKernelDispatchEvent()
	evt.SetTime(d.Freq.NextTick(req.RecvTime()))
	evt.SetHandler(d)
	evt.Status = status
	d.engine.Schedule(evt)

	return nil
}

func (d *Dispatcher) processWGFinishWGMesg(mesg *WGFinishMesg) *core.Error {
	status := mesg.Status

	status.CompletedWGs = append(status.CompletedWGs, mesg.WG)

	if len(status.CompletedWGs) == len(status.Grid.WorkGroups) {
		status.Req.SwapSrcAndDst()
		d.GetConnection("ToCommandProcessor").Send(status.Req)
	}

	return nil
}

// Handle perform actions when an event is triggered
//
// Dispatcher processes
//     KernalDispatchEvent ---- continues the kernel dispatching process.
//
func (d *Dispatcher) Handle(evt core.Event) error {
	switch e := evt.(type) {
	case *KernelDispatchEvent:
		d.handleKernelDispatchEvent(e)
	default:
		log.Panicf("Unable to process evevt %+v", evt)
	}
	return nil
}

func (d *Dispatcher) handleKernelDispatchEvent(evt *KernelDispatchEvent) error {
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
	entryPoint := status.Grid.Packet.KernelObject +
		status.Grid.CodeObject.KernelCodeEntryByteOffset

	var wf *kernels.Wavefront
	var info *WfDispatchInfo
	var req *DispatchWfReq
	for wf, info = range status.DispatchingWfs {
		req = NewDispatchWfReq(d, d.CUs[status.DispatchingCUID], evt.Time(),
			wf, info, entryPoint)
		req.CodeObject = status.CodeObject
		req.Packet = status.Packet
		break
	}

	err := d.GetConnection("ToCUs").Send(req)
	if err != nil && err.Recoverable {
		log.Panic(err)
	} else if err != nil {
		evt.SetTime(d.Freq.NoEarlierThan(err.EarliestRetry))
		d.engine.Schedule(evt)
	} else {
		delete(status.DispatchingWfs, wf)
		if len(status.DispatchingWfs) == 0 {
			status.Mapped = false
		}
		if len(status.DispatchingWfs) > 0 || len(status.WGs) > 0 {
			evt.SetTime(d.Freq.NextTick(evt.Time()))
			d.engine.Schedule(evt)
		}
	}
}

func (d *Dispatcher) mapWG(evt *KernelDispatchEvent) {
	status := evt.Status
	if len(status.WGs) != 0 && !d.isAllCUsBusy(status) {
		cuID := d.nextAvailableCU(status)
		cu := d.CUs[cuID]
		wg := status.WGs[0]
		req := NewMapWGReq(d, cu, evt.Time(), wg, status)
		req.CUID = cuID

		log.Printf("Trying to map wg to cu %d\n", cuID)
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
	count := len(status.CUBusy)
	cuID := status.DispatchingCUID
	for i := 0; i < count; i++ {
		cuID++
		if cuID >= len(status.CUBusy) {
			cuID = 0
		}

		if !status.CUBusy[cuID] {
			return cuID
		}
	}
	return -1
}
