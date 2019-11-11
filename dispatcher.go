package gcn3

import (
	"fmt"
	"log"

	mpb "github.com/vbauerster/mpb/v4"
	"github.com/vbauerster/mpb/v4/decor"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/util/tracing"
)

// dispatcherState defines the current state of the dispatcher
type dispatcherState int

// A list of all possible dispatcher states
const (
	dispatcherIdle dispatcherState = iota
	dispatcherToMapWG
	dispatcherWaitMapWGACK
)

var barGroup *mpb.Progress

func init() {
	barGroup = mpb.New()
}

// A Dispatcher is a component that can dispatch work-groups and wavefronts
// to ComputeUnits.
//
//     <=> ToCUs The connection that is connecting the dispatcher and the
//         compute units
//
//     <=> ToCP The connection that is connecting the dispatcher
//         with the command processor
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
//                            to the compute units (Initialize)
//
//     MapWGReq ---- The request return from the compute unit tells if the
//                   compute unit is able to run the work-group (Receive(?))
//
//     WGFinishMesg ---- The CU send this message to the dispatcher to notify
//                       the completion of a workgroup (Finalization(?))
//
type Dispatcher struct {
	*akita.TickingComponent

	CUs    []akita.Port
	cuBusy map[akita.Port]bool

	gridBuilder kernels.GridBuilder

	// The request that is being processed, one dispatcher can only dispatch one kernel at a time.
	dispatchingReq  *LaunchKernelReq
	totalWGs        int
	currentWG       *kernels.WorkGroup
	dispatchedWGs   map[string]*MapWGReq
	completedWGs    []*kernels.WorkGroup
	dispatchingWfs  []*kernels.Wavefront
	dispatchingCUID int
	state           dispatcherState
	progressBar     *mpb.Bar

	ToCUs              akita.Port
	ToCommandProcessor akita.Port
}

func (d *Dispatcher) Tick(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = d.mapWG(now) || madeProgress
	madeProgress = d.processReqFromCP(now) || madeProgress
	madeProgress = d.processRspFromCU(now) || madeProgress
	madeProgress = d.replyKernelFinish(now) || madeProgress

	return madeProgress
}

func (d *Dispatcher) mapWG(now akita.VTimeInSec) bool {
	if d.state != dispatcherToMapWG {
		return false
	}

	wg := d.currentWG
	if wg == nil {
		wg = d.gridBuilder.NextWG()
		if wg == nil {
			d.state = dispatcherIdle
			return false
		}
		d.currentWG = wg
	}

	cuID, hasAvailableCU := d.nextAvailableCU()
	if !hasAvailableCU {
		d.state = dispatcherIdle
		return false
	}

	CU := d.CUs[cuID]
	req := NewMapWGReq(d.ToCUs, CU, now, wg)
	req.PID = d.dispatchingReq.PID
	d.state = dispatcherWaitMapWGACK
	err := d.ToCUs.Send(req)
	if err != nil {
		return false
	}

	d.dispatchedWGs[wg.UID] = req
	d.dispatchingCUID = cuID

	tracing.TraceReqInitiate(
		req, now, d,
		tracing.MsgIDAtReceiver(d.dispatchingReq, d))

	return true
}

func (d *Dispatcher) processReqFromCP(now akita.VTimeInSec) bool {
	msg := d.ToCommandProcessor.Peek()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *LaunchKernelReq:
		return d.processLaunchKernelReq(now, req)
	}

	panic("never")
}

func (d *Dispatcher) processRspFromCU(now akita.VTimeInSec) bool {
	msg := d.ToCUs.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *MapWGReq:
		return d.processMapWGRsp(now, msg)
	case *WGFinishMesg:
		return d.processWGFinishMesg(now, msg)
	}

	panic("never")
}

func (d *Dispatcher) processLaunchKernelReq(
	now akita.VTimeInSec,
	req *LaunchKernelReq,
) bool {
	if d.dispatchingReq != nil {
		log.Panic("dispatcher not done dispatching the previous kernel")
	}

	d.initKernelDispatching(now, req)
	d.ToCommandProcessor.Retrieve(now)

	tracing.TraceReqReceive(req, now, d)

	return true
}

func (d *Dispatcher) replyLaunchKernelReq(
	ok bool,
	req *LaunchKernelReq,
	now akita.VTimeInSec,
) *akita.SendError {
	req.OK = ok
	req.Src, req.Dst = req.Dst, req.Src
	req.SendTime = req.RecvTime
	return d.ToCommandProcessor.Send(req)
}

func (d *Dispatcher) initKernelDispatching(
	now akita.VTimeInSec,
	req *LaunchKernelReq,
) {
	d.dispatchingReq = req
	d.gridBuilder.SetKernel(kernels.KernelLaunchInfo{
		CodeObject: req.HsaCo,
		Packet:     req.Packet,
		PacketAddr: req.PacketAddress,
	})
	d.totalWGs = d.gridBuilder.NumWG()
	d.dispatchingCUID = -1
	d.state = dispatcherToMapWG

	d.progressBar = barGroup.AddBar(
		int64(d.totalWGs),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("At %s, Kernel: %s, ", d.Name(), req.ID)),
			decor.Counters(0, "%d/%d"),
		),
		mpb.AppendDecorators(
			decor.Percentage(),
			decor.AverageSpeed(0, " %.2f/s, "),
			decor.AverageETA(decor.ET_STYLE_HHMMSS),
		),
	)
	d.progressBar.SetTotal(int64(d.totalWGs), false)

	tracing.TraceReqReceive(req, now, d)
}

func (d *Dispatcher) processMapWGRsp(
	now akita.VTimeInSec,
	rsp *MapWGReq,
) bool {
	if !rsp.Ok {
		return d.processFailedMapWGRsp(now, rsp)
	}
	return d.processSuccessfulMapWGRsp(now, rsp)
}

func (d *Dispatcher) processFailedMapWGRsp(
	now akita.VTimeInSec,
	rsp *MapWGReq,
) bool {
	d.state = dispatcherToMapWG
	d.cuBusy[d.CUs[d.dispatchingCUID]] = true

	delete(d.dispatchedWGs, d.currentWG.UID)
	d.ToCUs.Retrieve(now)

	tracing.TraceReqReceive(rsp, now, d)

	return true
}

func (d *Dispatcher) processSuccessfulMapWGRsp(
	now akita.VTimeInSec,
	rsp *MapWGReq,
) bool {
	d.currentWG = nil
	d.state = dispatcherToMapWG
	d.ToCUs.Retrieve(now)
	return true
}

func (d *Dispatcher) processWGFinishMesg(
	now akita.VTimeInSec,
	msg *WGFinishMesg,
) bool {
	d.ToCUs.Retrieve(now)
	d.completedWGs = append(d.completedWGs, msg.WG)
	d.cuBusy[msg.Src] = false

	mapWGReq := d.dispatchedWGs[msg.WG.UID]
	delete(d.dispatchedWGs, msg.WG.UID)

	tracing.TraceReqFinalize(mapWGReq, now, d)

	if d.progressBar != nil {
		d.progressBar.Increment()
	}

	if d.state == dispatcherIdle {
		d.state = dispatcherToMapWG
	}
	return true
}

func (d *Dispatcher) replyKernelFinish(now akita.VTimeInSec) bool {
	if d.dispatchingReq == nil {
		return false
	}

	if len(d.completedWGs) < d.totalWGs {
		return false
	}

	req := d.dispatchingReq
	req.Src, req.Dst = req.Dst, req.Src
	req.SendTime = now
	err := d.ToCommandProcessor.Send(req)
	if err != nil {
		return false
	}

	d.completedWGs = nil
	d.dispatchingReq = nil

	tracing.TraceReqComplete(req, now, d)

	return true
}

// RegisterCU adds a CU to the dispatcher so that the dispatcher can
// dispatches wavefronts to the CU
func (d *Dispatcher) RegisterCU(cu akita.Port) {
	d.CUs = append(d.CUs, cu)
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

		if !d.cuBusy[d.CUs[cuID]] {
			return cuID, true
		}
	}
	return -1, false
}

// NewDispatcher creates a new dispatcher
func NewDispatcher(
	name string,
	engine akita.Engine,
	gridBuilder kernels.GridBuilder,
) *Dispatcher {
	d := new(Dispatcher)
	d.TickingComponent = akita.NewTickingComponent(name, engine, 1*akita.GHz, d)

	d.gridBuilder = gridBuilder

	d.CUs = make([]akita.Port, 0)
	d.cuBusy = make(map[akita.Port]bool, 0)
	d.dispatchedWGs = make(map[string]*MapWGReq)

	d.ToCommandProcessor = akita.NewLimitNumMsgPort(d, 1,
		name+".ToCommandProcessor")
	d.ToCUs = akita.NewLimitNumMsgPort(d, 1, name+".ToCUs")

	d.state = dispatcherIdle

	return d
}
