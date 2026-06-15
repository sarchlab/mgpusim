package dispatching

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/daisen2"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/monitoring2"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// A PortSource provides ports by name. The Command Processor component
// satisfies this interface; dispatchers resolve the ports they use lazily,
// since port instances are assigned to the component after Build.
type PortSource interface {
	GetPortByName(name string) messaging.Port
}

// A Dispatcher is a sub-component of a command processor that can dispatch
// work-groups to compute units.
type Dispatcher interface {
	tracing.NamedHookable
	RegisterCU(cu resource.DispatchableCU)
	IsDispatching() bool
	StartDispatching(req protocol.LaunchKernelReq)
	Tick() (madeProgress bool)
}

// A DispatcherImpl is a ticking component that can dispatch work-groups.
//
// TODO(akita5): state purity — the dispatcher is a sub-object of the Command
// Processor and keeps its runtime state (in-flight work-group maps holding
// messages, the algorithm object graph) in plain fields. It is not
// checkpointable.
type DispatcherImpl struct {
	hooking.HookableBase

	cp                  tracing.NamedHookable
	name                string
	portSource          PortSource
	respondingPortName  string
	dispatchingPortName string
	respondingPort      messaging.Port
	dispatchingPort     messaging.Port

	alg                            algorithm
	dispatching                    protocol.LaunchKernelReq
	isDispatching                  bool
	currWG                         dispatchLocation
	cycleLeft                      int
	numDispatchedWGs               int
	numCompletedWGs                int
	inflightWGs                    map[uint64]dispatchLocation
	originalReqs                   map[uint64]protocol.MapWGReq
	latencyTable                   []int
	constantKernelOverhead         int
	constantKernelLaunchOverhead   int
	subsequentKernelLaunchOverhead int
	firstKernelLaunched            bool
	prevKernelWGCount              int
	wgScalingThreshold             int

	monitor     *monitoring2.Monitor
	progressBar *daisen2.ProgressBar
}

// Name returns the name of the dispatcher
func (d *DispatcherImpl) Name() string {
	return d.name
}

// CurrentTime returns the current time, as told by the Command Processor that
// the dispatcher belongs to.
func (d *DispatcherImpl) CurrentTime() timing.VTimeInPicoSec {
	return d.cp.CurrentTime()
}

func (d *DispatcherImpl) getDispatchingPort() messaging.Port {
	if d.dispatchingPort == nil {
		d.dispatchingPort = d.portSource.GetPortByName(d.dispatchingPortName)
	}

	return d.dispatchingPort
}

func (d *DispatcherImpl) getRespondingPort() messaging.Port {
	if d.respondingPort == nil {
		d.respondingPort = d.portSource.GetPortByName(d.respondingPortName)
	}

	return d.respondingPort
}

// RegisterCU allows the dispatcher to dispatch work-groups to the CU.
func (d *DispatcherImpl) RegisterCU(cu resource.DispatchableCU) {
	d.alg.RegisterCU(cu)
}

// IsDispatching checks if the dispatcher is dispatching another kernel.
func (d *DispatcherImpl) IsDispatching() bool {
	return d.isDispatching
}

// StartDispatching lets the dispatcher to start dispatch another kernel.
func (d *DispatcherImpl) StartDispatching(req protocol.LaunchKernelReq) {
	d.mustNotBeDispatchingAnotherKernel()

	d.alg.StartNewKernel(kernels.KernelLaunchInfo{
		CodeObject: req.CodeObject,
		Packet:     req.Packet,
		PacketAddr: req.PacketAddress,
		WGFilter:   req.WGFilter,
	})
	d.dispatching = req
	d.isDispatching = true

	d.numDispatchedWGs = 0
	d.numCompletedWGs = 0
	if !d.firstKernelLaunched {
		d.cycleLeft = d.constantKernelLaunchOverhead
		d.firstKernelLaunched = true
	} else {
		if d.prevKernelWGCount > 0 && d.wgScalingThreshold > 0 {
			scale := float64(d.wgScalingThreshold) / float64(d.prevKernelWGCount)
			d.cycleLeft = int(float64(d.subsequentKernelLaunchOverhead) * scale)
		} else {
			d.cycleLeft = d.subsequentKernelLaunchOverhead
		}
	}

	d.initializeProgressBar(req.ID)
}

func (d *DispatcherImpl) initializeProgressBar(kernelID uint64) {
	if d.monitor != nil {
		d.progressBar = d.monitor.CreateProgressBar(
			fmt.Sprintf("At %s, Kernel: %d, ", d.Name(), kernelID),
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

	if d.isDispatching {
		if d.kernelCompleted() {
			madeProgress = d.completeKernel() || madeProgress
		} else {
			// Dispatch up to 8 WGs per cycle
			for i := 0; i < 8; i++ {
				progress := d.dispatchNextWG()
				madeProgress = progress || madeProgress
				if !progress || d.cycleLeft > 0 {
					break
				}
			}
		}
	}

	madeProgress = d.processMessagesFromCU() || madeProgress

	return madeProgress
}

func (d *DispatcherImpl) collectSamplingData(
	locations []protocol.WfDispatchLocation,
) {
	if *sampling.SampledRunnerFlag {
		for _, l := range locations {
			wavefront := l.Wavefront
			sampling.SampledEngineInstance.Collect(
				wavefront.IssueTime, wavefront.FinishTime)
		}
	}
}

func (d *DispatcherImpl) processMessagesFromCU() bool {
	madeProgress := false

	for i := 0; i < 8; i++ {
		msg := d.getDispatchingPort().PeekIncoming()
		if msg == nil {
			break
		}

		switch msg := msg.(type) {
		case protocol.WGCompletionMsg:
			count := 0
			for _, rspToID := range msg.RspToIDs {
				location, ok := d.inflightWGs[rspToID]
				if ok {
					count++
					///sampling
					d.collectSamplingData(location.locations)
				}
			}

			if count == 0 {
				return madeProgress
			} else if count < len(msg.RspToIDs) {
				log.Panic(
					"all finished WGs must be from the same dispatcher")
			}

			for _, rspToID := range msg.RspToIDs {
				location := d.inflightWGs[rspToID]
				d.alg.FreeResources(location)
				delete(d.inflightWGs, rspToID)
				d.numCompletedWGs++
				if d.numCompletedWGs == d.alg.NumWG() {
					d.cycleLeft = d.constantKernelOverhead
				}

				originalReq := d.originalReqs[rspToID]
				delete(d.originalReqs, rspToID)
				tracing.TraceReqFinalize(d, originalReq)

				if d.progressBar != nil {
					d.progressBar.MoveInProgressToFinished(1)
				}
			}

			d.getDispatchingPort().RetrieveIncoming()
			madeProgress = true
		default:
			// Unknown message type, stop processing
			return madeProgress
		}
	}

	return madeProgress
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

	port := d.getRespondingPort()
	if !port.CanSend() {
		return false
	}

	rsp := protocol.LaunchKernelRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   port.AsRemote(),
			Dst:   req.Src,
			RspTo: req.ID,
		},
	}
	port.Send(rsp)

	d.prevKernelWGCount = d.numDispatchedWGs
	d.dispatching = protocol.LaunchKernelReq{}
	d.isDispatching = false

	if d.monitor != nil {
		d.monitor.CompleteProgressBar(d.progressBar)
	}

	tracing.TraceReqComplete(d.cp, req)

	return true
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

	port := d.getDispatchingPort()
	if !port.CanSend() {
		return false
	}

	req := protocol.MapWGReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: port.AsRemote(),
			Dst: d.currWG.cu,
		},
		WorkGroup:  d.currWG.wg,
		PID:        d.dispatching.PID,
		Wavefronts: d.currWG.locations,
	}
	port.Send(req)

	d.currWG.valid = false
	d.numDispatchedWGs++
	d.inflightWGs[req.ID] = d.currWG
	d.originalReqs[req.ID] = req
	d.cycleLeft = d.latencyTable[len(d.currWG.locations)]

	if d.progressBar != nil {
		d.progressBar.IncrementInProgress(1)
	}

	tracing.TraceReqInitiate(d, req,
		tracing.MsgIDAtReceiver(d.dispatching, d.cp))

	return true
}
