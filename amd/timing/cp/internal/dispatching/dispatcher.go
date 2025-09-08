package dispatching

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/sampling"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp/internal/resource"
)

// A Dispatcher is a sub-component of a command processor that can dispatch
// work-groups to compute units.
type Dispatcher interface {
	tracing.NamedHookable
	RegisterCU(cu resource.DispatchableCU)
	IsDispatching() bool
	StartDispatching(req *protocol.LaunchKernelReq)
	Tick() (madeProgress bool)
}

// A DispatcherImpl is a ticking component that can dispatch work-groups.
type DispatcherImpl struct {
	sim.HookableBase

	cp                     tracing.NamedHookable
	name                   string
	respondingPort         sim.Port
	dispatchingPort        sim.Port
	alg                    algorithm
	dispatching            *protocol.LaunchKernelReq
	currWG                 dispatchLocation
	cycleLeft              int
	numDispatchedWGs       int
	numCompletedWGs        int
	inflightWGs            map[string]dispatchLocation
	originalReqs           map[string]*protocol.MapWGReq
	latencyTable           []int
	constantKernelOverhead int

	monitor     *monitoring.Monitor
	progressBar *monitoring.ProgressBar
}

// Name returns the name of the dispatcher
func (d *DispatcherImpl) Name() string {
	return d.name
}

// RegisterCU allows the dispatcher to dispatch work-groups to the CU.
func (d *DispatcherImpl) RegisterCU(cu resource.DispatchableCU) {
	d.alg.RegisterCU(cu)
}

// IsDispatching checks if the dispatcher is dispatching another kernel.
func (d *DispatcherImpl) IsDispatching() bool {
	return d.dispatching != nil
}

// StartDispatching lets the dispatcher to start dispatch another kernel.
func (d *DispatcherImpl) StartDispatching(req *protocol.LaunchKernelReq) {
	d.mustNotBeDispatchingAnotherKernel()

	// removed verbose debug logging

	d.alg.StartNewKernel(kernels.KernelLaunchInfo{
		CodeObject: req.HsaCo,
		Packet:     req.Packet,
		PacketAddr: req.PacketAddress,
		WGFilter:   req.WGFilter,
	})
	d.dispatching = req

	d.numDispatchedWGs = 0
	d.numCompletedWGs = 0

	d.initializeProgressBar(req.ID)
}

func (d *DispatcherImpl) initializeProgressBar(kernelID string) {
	if d.monitor != nil {
		d.progressBar = d.monitor.CreateProgressBar(
			fmt.Sprintf("At %s, Kernel: %s, ", d.Name(), kernelID),
			uint64(d.alg.NumWG()),
		)
	}
}

func (d *DispatcherImpl) mustNotBeDispatchingAnotherKernel() {
	if d.IsDispatching() {
		panic("dispatcher is dispatching another request")
	}
}

// Tick updates the state of the dispatcher.
func (d *DispatcherImpl) Tick() (madeProgress bool) {
	if d.cycleLeft > 0 {
		d.cycleLeft--
		return true
	}

	if d.dispatching != nil {
		if d.kernelCompleted() {
			madeProgress = d.completeKernel() || madeProgress
		} else {
			madeProgress = d.dispatchNextWG() || madeProgress
		}
	}

	madeProgress = d.processMessagesFromCU() || madeProgress

	return madeProgress
}

func (d *DispatcherImpl) collectSamplingData(locations []protocol.WfDispatchLocation) {
	if *sampling.SampledRunnerFlag {
		for _, l := range locations {
			wavefront := l.Wavefront
			sampling.SampledEngineInstance.Collect(
				wavefront.IssueTime, wavefront.FinishTime)
		}
	}
}

func (d *DispatcherImpl) processMessagesFromCU() bool {
	msg := d.dispatchingPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *protocol.WGCompletionMsg:
		count := 0
		for _, rspToID := range msg.RspTo {
			location, ok := d.inflightWGs[rspToID]
			if ok {
				count += 1
				///sampling
				d.collectSamplingData(location.locations)
			}
		}

		if count == 0 {
			return false
		} else if count < len(msg.RspTo) {
			log.Panic("In emulation all finished WGs from more than one dispatcher")
		}

		for _, rspToID := range msg.RspTo {
			location := d.inflightWGs[rspToID]
			d.alg.FreeResources(location)
			delete(d.inflightWGs, rspToID)
			d.numCompletedWGs++
			if d.numCompletedWGs == d.alg.NumWG() {
				d.cycleLeft = d.constantKernelOverhead
			}

			originalReq := d.originalReqs[rspToID]
			delete(d.originalReqs, rspToID)
			tracing.TraceReqFinalize(originalReq, d)

			if d.progressBar != nil {
				d.progressBar.MoveInProgressToFinished(1)
			}
		}

		d.dispatchingPort.RetrieveIncoming()
		return true
	}

	return false
}

func (d *DispatcherImpl) kernelCompleted() bool {
	if d.currWG.valid {
		return false
	}

	if d.alg.HasNext() {
		return false
	}

	if d.numCompletedWGs < d.numDispatchedWGs {
		return false
	}

	return true
}

func (d *DispatcherImpl) completeKernel() (
	madeProgress bool,
) {
	req := d.dispatching

	rsp := protocol.NewLaunchKernelRsp(req.Dst, req.Src, req.ID)

	err := d.respondingPort.Send(rsp)
	if err == nil {
		d.dispatching = nil

		if d.monitor != nil {
			d.monitor.CompleteProgressBar(d.progressBar)
		}

		tracing.TraceReqComplete(req, d.cp)

		return true
	}

	return false
}

func (d *DispatcherImpl) dispatchNextWG() (madeProgress bool) {
	if !d.currWG.valid {
		if !d.alg.HasNext() {
			return false
		}
		d.currWG = d.alg.Next()
		if !d.currWG.valid {
			return false
		}
	}

	reqBuilder := protocol.MapWGReqBuilder{}.
		WithSrc(d.dispatchingPort.AsRemote()).
		WithDst(d.currWG.cu).
		WithPID(d.dispatching.PID).
		WithWG(d.currWG.wg)
	for _, l := range d.currWG.locations {
		reqBuilder = reqBuilder.AddWf(l)
	}
	req := reqBuilder.Build()
	err := d.dispatchingPort.Send(req)
	if err != nil {
		return false
	}

	// fmt.Printf("%.10f, %d, %d\n", now, d.currWG.wg.IDX, d.currWG.cuID)

	if err == nil {
		d.currWG.valid = false
		d.numDispatchedWGs++
		d.inflightWGs[req.ID] = d.currWG
		d.originalReqs[req.ID] = req
		d.cycleLeft = d.latencyTable[len(d.currWG.locations)]

		if d.progressBar != nil {
			d.progressBar.IncrementInProgress(1)
		}

		tracing.TraceReqInitiate(req, d,
			tracing.MsgIDAtReceiver(d.dispatching, d.cp))

		return true
	}

	return false
}
