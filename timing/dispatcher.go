package timing

import (
	"log"
	"reflect"

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
	Ok            bool
	WfDispatchMap map[*kernels.Wavefront]*WfDispatchInfo // Tells where a wf should fit in
	CUID          int
	CodeObject    *insts.HsaCo
}

// NewMapWGReq returns a newly created MapWGReq
func NewMapWGReq(
	src, dst core.Component,
	time core.VTimeInSec,
	wg *kernels.WorkGroup,
	co *insts.HsaCo,
) *MapWGReq {
	r := new(MapWGReq)
	r.ReqBase = core.NewReqBase()
	r.SetSrc(src)
	r.SetDst(dst)
	r.SetSendTime(time)
	r.WG = wg
	r.CodeObject = co
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
	*core.ComponentBase

	CUs  []core.Component
	Freq core.Freq

	engine            core.Engine
	gridBuilder       kernels.GridBuilder
	dispatchingKernel *KernelDispatchStatus
	running           bool
}

// NewDispatcher creates a new dispatcher
func NewDispatcher(
	name string,
	engine core.Engine,
	gridBuilder kernels.GridBuilder,
) *Dispatcher {
	d := new(Dispatcher)
	d.ComponentBase = core.NewComponentBase(name)
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

	d.InvokeHook(req, d, core.OnRecvReq, nil)

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

	// FIXME: Rather than processing the request, dispatcher should retrieve
	// request from some queue
	if d.dispatchingKernel != nil {
		err := core.NewError("Cannot Dispatch", true,
			d.Freq.NCyclesLater(10, req.RecvTime()))
		return err
	}

	d.initStatus(req)
	d.tryScheduleTick(d.Freq.NextTick(req.RecvTime()))
	return nil
}

func (d *Dispatcher) initStatus(req *kernels.LaunchKernelReq) {
	status := NewKernelDispatchStatus()
	status.Req = req
	status.Packet = req.Packet
	status.Grid = d.gridBuilder.Build(req)
	status.WGs = append(status.WGs, status.Grid.WorkGroups...)
	status.DispatchingCUID = -1
	status.CodeObject = req.HsaCo
	for range d.CUs {
		status.CUBusy = append(status.CUBusy, false)
	}
	d.dispatchingKernel = status
}

func (d *Dispatcher) tryScheduleTick(t core.VTimeInSec) {
	if !d.running {
		d.scheduleTick(t)
	}
}

func (d *Dispatcher) scheduleTick(t core.VTimeInSec) {
	evt := core.NewTickEvent(t, d)
	d.engine.Schedule(evt)
	d.running = true

}

func (d *Dispatcher) processMapWGReq(req *MapWGReq) *core.Error {
	status := d.dispatchingKernel

	if req.Ok {
		for i, wgToDel := range status.WGs {
			if wgToDel == req.WG {
				status.WGs = append(status.WGs[:i], status.WGs[i+1:]...)
			}
		}
		status.DispatchingWfs = req.WfDispatchMap
		status.DispatchingCUID = req.CUID
		status.Mapped = true
	} else {
		log.Printf("Marking cu %d as busy\n", req.CUID)
		status.CUBusy[req.CUID] = true
	}

	d.tryScheduleTick(d.Freq.NextTick(req.RecvTime()))
	return nil
}

func (d *Dispatcher) processWGFinishWGMesg(mesg *WGFinishMesg) *core.Error {
	status := d.dispatchingKernel
	status.CompletedWGs = append(status.CompletedWGs, mesg.WG)

	if len(status.CompletedWGs) == len(status.Grid.WorkGroups) {
		status.Req.SwapSrcAndDst()
		d.GetConnection("ToCommandProcessor").Send(status.Req)
	} else {
		status.CUBusy[mesg.CUID] = false
		d.tryScheduleTick(d.Freq.NextTick(d.Freq.NextTick(mesg.RecvTime())))
	}

	return nil
}

// Handle perform actions when an event is triggered
//
// Dispatcher processes
//     KernalDispatchEvent ---- continues the kernel dispatching process.
//
func (d *Dispatcher) Handle(evt core.Event) error {
	d.Lock()
	defer d.Unlock()

	d.InvokeHook(evt, d, core.BeforeEvent, nil)
	defer d.InvokeHook(evt, d, core.AfterEvent, nil)

	switch e := evt.(type) {
	case *core.TickEvent:
		d.handleTickEvent(e)
	default:
		log.Panicf("Unable to process evevt of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (d *Dispatcher) handleTickEvent(evt *core.TickEvent) error {
	status := d.dispatchingKernel
	if status.Mapped {
		d.dispatchWf(evt.Time())
	} else {
		d.mapWG(evt.Time())
	}

	return nil
}

func (d *Dispatcher) dispatchWf(now core.VTimeInSec) {
	status := d.dispatchingKernel
	entryPoint := status.Grid.Packet.KernelObject +
		status.Grid.CodeObject.KernelCodeEntryByteOffset

	var wf *kernels.Wavefront
	var info *WfDispatchInfo
	var req *DispatchWfReq
	for wf, info = range status.DispatchingWfs {
		req = NewDispatchWfReq(d, d.CUs[status.DispatchingCUID], now,
			wf, info, entryPoint)
		req.CodeObject = status.CodeObject
		req.Packet = status.Packet
		break
	}

	err := d.GetConnection("ToCUs").Send(req)
	if err != nil && err.Recoverable {
		log.Panic(err)
	} else if err != nil {
		d.scheduleTick(d.Freq.NoEarlierThan(err.EarliestRetry))
	} else {
		delete(status.DispatchingWfs, wf)
		if len(status.DispatchingWfs) == 0 {
			status.Mapped = false
		}
		if len(status.DispatchingWfs) > 0 || len(status.WGs) > 0 {
			d.scheduleTick(d.Freq.NextTick(now))
		}
	}
}

func (d *Dispatcher) mapWG(now core.VTimeInSec) {
	status := d.dispatchingKernel
	if len(status.WGs) != 0 && !d.isAllCUsBusy(status) {
		cuID := d.nextAvailableCU(status)
		cu := d.CUs[cuID]
		wg := status.WGs[0]
		req := NewMapWGReq(d, cu, now, wg, status.CodeObject)
		req.CUID = cuID

		log.Printf("Trying to map wg to cu %d\n", cuID)
		d.GetConnection("ToCUs").Send(req)
	}

	// Always pause the dispatching and wait for the reply to determine whether
	// to continue
	d.running = false
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
