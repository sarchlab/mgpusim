package dispatching

import (
	"fmt"
	"log"

	daisen2 "github.com/sarchlab/akita/v5/monitoring"
	monitoring2 "github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
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
	Name() string
	CurrentTime() timing.VTimeInPicoSec
	TaskTracker() *tasks.Tracker
	RegisterCU(cu resource.DispatchableCU)
	IsDispatching() bool
	StartDispatching(req messaging.Msg)
	Tick() (madeProgress bool)
}

// A DispatcherImpl is a ticking component that can dispatch work-groups.
//
// Its checkpoint is registered separately from the parent component so each
// dispatcher retains its algorithm cursors, reservations, and task handles.
type DispatcherImpl struct {
	sampler *sampling.SampledEngine
	hooking.HookableBase

	cp                  Parent
	name                string
	portSource          PortSource
	respondingPortName  string
	dispatchingPortName string
	respondingPort      messaging.Port
	dispatchingPort     messaging.Port

	alg                            algorithm
	dieAware                       dieAwareAlgorithm // non-nil iff alg dispatches per-die
	dieCyclesLeft                  []int             // per-die rate gate (cycles until next dispatch)
	wavefrontDispatchCycles        int               // per-die cost charged per wavefront dispatched
	dispatching                    messaging.Msg
	isDispatching                  bool
	currWG                         dispatchLocation
	cycleLeft                      int
	numDispatchedWGs               int
	numCompletedWGs                int
	inflightWGs                    map[uint64]dispatchLocation
	wgTaskIDs                      map[uint64]uint64
	launchTaskID                   uint64
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
func (d *DispatcherImpl) StartDispatching(req messaging.Msg) {
	d.mustNotBeDispatchingAnotherKernel()

	payload := req.Payload.(protocol.LaunchKernelReq)
	d.alg.StartNewKernel(
		kernels.KernelLaunchInfo{
			CodeObject: payload.CodeObject.Restore(),
			Packet:     &payload.Packet,
			PacketAddr: payload.PacketAddress,
			Selection:  payload.Selection,
		},
	)
	d.launchTaskID = d.cp.TaskTracker().StartReqIn(d.cp, req)

	d.dispatching = req
	d.isDispatching = true

	d.numDispatchedWGs = 0
	d.numCompletedWGs = 0
	for i := range d.dieCyclesLeft {
		d.dieCyclesLeft[i] = 0
	}
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
		} else if d.dieAware != nil {
			madeProgress = d.tickPerDie() || madeProgress
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
	times []protocol.WavefrontTiming,
) {
	if d.sampler == nil || d.sampler.IfDisable() {
		return
	}
	for _, l := range locations {
		var issue, finish timing.VTimeInPicoSec
		for _, sample := range times {
			if sample.WavefrontID == l.WavefrontID {
				issue, finish = sample.IssueTime, sample.FinishTime
				break
			}
		}
		d.sampler.Collect(issue, finish)
	}
}

func (d *DispatcherImpl) processMessagesFromCU() bool {
	madeProgress := false

	for i := 0; i < 8; i++ {
		msg, ok := d.getDispatchingPort().PeekIncoming()
		if !ok {
			break
		}

		switch msg.Payload.(type) {
		case protocol.WGCompletionMsg:

			count := 0
			for _, rspToID := range msg.Payload.(protocol.WGCompletionMsg).RspToIDs {
				location, ok := d.inflightWGs[rspToID]
				if ok {
					count++
					///sampling
					d.collectSamplingData(location.locations, msg.Payload.(protocol.WGCompletionMsg).Timings)
				}
			}

			if count == 0 {
				return madeProgress
			} else if count < len(msg.Payload.(protocol.WGCompletionMsg).RspToIDs) {
				log.Panic(
					"all finished WGs must be from the same dispatcher")
			}

			for _, rspToID := range msg.Payload.(protocol.WGCompletionMsg).RspToIDs {
				location := d.inflightWGs[rspToID]
				d.alg.FreeResources(location)
				delete(d.inflightWGs, rspToID)
				d.numCompletedWGs++
				if d.numCompletedWGs == d.alg.NumWG() {
					d.cycleLeft = d.constantKernelOverhead
				}

				d.cp.TaskTracker().EndReqOut(d.wgTaskIDs[rspToID])
				delete(d.wgTaskIDs, rspToID)

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

	rsp := messaging.Msg{ID: d.cp.NewID(),
		Src:   port.AsRemote(),
		Dst:   req.Src,
		RspTo: req.ID, Payload: protocol.LaunchKernelRsp{}}

	port.Send(rsp)

	d.prevKernelWGCount = d.numDispatchedWGs
	d.dispatching = messaging.Msg{Payload: protocol.LaunchKernelReq{}}
	d.isDispatching = false

	if d.monitor != nil {
		d.monitor.CompleteProgressBar(d.progressBar)
	}

	d.cp.TaskTracker().EndReqIn(d.launchTaskID)
	d.launchTaskID = 0

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

	req := messaging.Msg{ID: d.cp.NewID(),
		Src: port.AsRemote(),
		Dst: d.currWG.cu, Payload: protocol.MapWGReq{WorkGroup: kernels.WorkGroupSnapshot(d.currWG.wg),
			PID:        d.dispatching.Payload.(protocol.LaunchKernelReq).PID,
			Wavefronts: d.currWG.locations}}

	d.wgTaskIDs[req.ID] = d.cp.TaskTracker().StartReqOut(d, req, d.launchTaskID)
	port.Send(req)

	d.currWG.valid = false
	d.numDispatchedWGs++
	d.inflightWGs[req.ID] = d.currWG
	d.cycleLeft = d.latencyTable[len(d.currWG.locations)]

	if d.progressBar != nil {
		d.progressBar.IncrementInProgress(1)
	}

	// The MapWGReq req_out is opened on the dispatcher domain (d), while its
	// parent — the LaunchKernelReq req_in — lives on the Command Processor
	// domain (d.cp). This cross-domain parent link is intentional and resolves
	// correctly: the vis DBTracer is the same instance attached to both d (via
	// report.go / the CP builder) and d.cp (via RegisterComponent), so both
	// tasks land in one trace database keyed by task ID.

	return true
}

// tickPerDie advances all dies one cycle: each die whose rate gate is free may
// dispatch one work-group this cycle, so up to NumDies work-groups dispatch per
// cycle (one per die), each die independently throttled by wavefrontDispatchCycles.
func (d *DispatcherImpl) tickPerDie() (madeProgress bool) {
	for die := range d.dieCyclesLeft {
		if d.dieCyclesLeft[die] > 0 {
			d.dieCyclesLeft[die]--
			madeProgress = true
			continue
		}

		if d.dispatchNextWGForDie(die) {
			madeProgress = true
		}
	}

	return madeProgress
}

// dispatchNextWGForDie dispatches one work-group on the given die and arms that
// die's rate gate for wavefrontDispatchCycles per wavefront dispatched. The
// port's send capacity is checked before reserving CU resources so a full port
// never strands a reserved work-group.
func (d *DispatcherImpl) dispatchNextWGForDie(die int) (madeProgress bool) {
	port := d.getDispatchingPort()
	if !port.CanSend() {
		return false
	}

	loc := d.dieAware.NextForDie(die)
	if !loc.valid {
		return false
	}

	req := messaging.Msg{ID: d.cp.NewID(),
		Src: port.AsRemote(),
		Dst: loc.cu, Payload: protocol.MapWGReq{WorkGroup: kernels.WorkGroupSnapshot(loc.wg),
			PID:        d.dispatching.Payload.(protocol.LaunchKernelReq).PID,
			Wavefronts: loc.locations}}

	d.wgTaskIDs[req.ID] = d.cp.TaskTracker().StartReqOut(d, req, d.launchTaskID)
	port.Send(req)

	d.numDispatchedWGs++
	d.inflightWGs[req.ID] = loc
	// The die is busy dispatching this WG's wavefronts for
	// wavefrontDispatchCycles*W cycles total. This dispatch consumes the current
	// cycle (the first busy cycle), so the gate holds the remaining W*cycles-1,
	// giving an exact per-die period of W*cycles (not W*cycles+1).
	d.dieCyclesLeft[die] = d.wavefrontDispatchCycles*len(loc.locations) - 1

	if d.progressBar != nil {
		d.progressBar.IncrementInProgress(1)
	}

	return true
}

// Parent supplies the simulation services owned by the command processor.
type Parent interface {
	tasks.Owner
	CurrentTime() timing.VTimeInPicoSec
	NewID() uint64
	TaskTracker() *tasks.Tracker
}

func (d *DispatcherImpl) TaskTracker() *tasks.Tracker { return d.cp.TaskTracker() }
