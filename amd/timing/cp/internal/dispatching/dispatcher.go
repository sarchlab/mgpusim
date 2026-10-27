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
	IsDispatching() bool
	StartDispatching(req messaging.Msg)
	Tick() (madeProgress bool)
}

// A DispatcherImpl is a ticking component that can dispatch work-groups.
//
// All mutable dispatch data belongs to its parent component State.
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

	state                          func() *State
	alg                            algorithm
	dieAware                       dieAwareAlgorithm // non-nil iff alg dispatches per-die
	wavefrontDispatchCycles        int               // per-die cost charged per wavefront dispatched
	latencyTable                   []int
	constantKernelOverhead         int
	constantKernelLaunchOverhead   int
	subsequentKernelLaunchOverhead int
	wgScalingThreshold             int

	monitor       *monitoring2.Monitor
	progressBar   *daisen2.ProgressBar
	progressState *State
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

// IsDispatching checks if the dispatcher is dispatching another kernel.
func (d *DispatcherImpl) IsDispatching() bool {
	return d.state().IsDispatching
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
	d.state().LaunchTaskID = d.cp.TaskTracker().StartReqIn(d.cp, req)

	d.state().Dispatching = req
	d.state().IsDispatching = true

	d.state().NumDispatchedWGs = 0
	d.state().NumCompletedWGs = 0
	for i := range d.state().DieCyclesLeft {
		d.state().DieCyclesLeft[i] = 0
	}
	if !d.state().FirstKernelLaunched {
		d.state().CycleLeft = d.constantKernelLaunchOverhead
		d.state().FirstKernelLaunched = true
	} else {
		if d.state().PrevKernelWGCount > 0 && d.wgScalingThreshold > 0 {
			scale := float64(d.wgScalingThreshold) / float64(d.state().PrevKernelWGCount)
			d.state().CycleLeft = int(float64(d.subsequentKernelLaunchOverhead) * scale)
		} else {
			d.state().CycleLeft = d.subsequentKernelLaunchOverhead
		}
	}

	d.initializeProgressBar(req.ID)
}

func (d *DispatcherImpl) initializeProgressBar(kernelID uint64) {
	d.progressState = d.state()
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
	// Observational UI objects are reconstructed from canonical State after load.
	if d.monitor != nil && d.progressState != d.state() {
		d.progressBar = nil
		d.progressState = d.state()
		if d.state().IsDispatching {
			d.initializeProgressBar(d.state().Dispatching.ID)
			d.progressBar.IncrementInProgress(uint64(d.state().NumDispatchedWGs))
			d.progressBar.MoveInProgressToFinished(uint64(d.state().NumCompletedWGs))
		}
	}

	if d.state().CycleLeft > 0 {
		d.state().CycleLeft--
		return true
	}

	if d.state().IsDispatching {
		if d.kernelCompleted() {
			madeProgress = d.completeKernel() || madeProgress
		} else if d.dieAware != nil {
			madeProgress = d.tickPerDie() || madeProgress
		} else {
			// Dispatch up to 8 WGs per cycle
			for i := 0; i < 8; i++ {
				progress := d.dispatchNextWG()
				madeProgress = progress || madeProgress
				if !progress || d.state().CycleLeft > 0 {
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
				location, ok := d.state().Inflight[rspToID]
				if ok {
					count++
					///sampling
					d.collectSamplingData(location.Locations, msg.Payload.(protocol.WGCompletionMsg).Timings)
				}
			}

			if count == 0 {
				return madeProgress
			} else if count < len(msg.Payload.(protocol.WGCompletionMsg).RspToIDs) {
				log.Panic(
					"all finished WGs must be from the same dispatcher")
			}

			for _, rspToID := range msg.Payload.(protocol.WGCompletionMsg).RspToIDs {
				location := d.state().Inflight[rspToID]
				d.alg.FreeResources(mustRestore(location))
				delete(d.state().Inflight, rspToID)
				d.state().NumCompletedWGs++
				if d.state().NumCompletedWGs == d.alg.NumWG() {
					d.state().CycleLeft = d.constantKernelOverhead
				}

				d.cp.TaskTracker().EndReqOut(d.state().WgTaskIDs[rspToID])
				delete(d.state().WgTaskIDs, rspToID)

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
	if d.state().Current.Valid {
		return false
	}

	if d.alg.HasNext() {
		return false
	}

	if d.state().NumCompletedWGs < d.state().NumDispatchedWGs {
		return false
	}

	return true
}

func (d *DispatcherImpl) completeKernel() (
	madeProgress bool,
) {
	req := d.state().Dispatching

	port := d.getRespondingPort()
	if !port.CanSend() {
		return false
	}

	rsp := messaging.Msg{ID: d.cp.NewID(),
		Src:   port.AsRemote(),
		Dst:   req.Src,
		RspTo: req.ID, Payload: protocol.LaunchKernelRsp{}}

	port.Send(rsp)

	d.state().PrevKernelWGCount = d.state().NumDispatchedWGs
	d.state().Dispatching = messaging.Msg{Payload: protocol.LaunchKernelReq{}}
	d.state().IsDispatching = false

	if d.monitor != nil {
		d.monitor.CompleteProgressBar(d.progressBar)
	}

	d.cp.TaskTracker().EndReqIn(d.state().LaunchTaskID)
	d.state().LaunchTaskID = 0

	return true
}

func (d *DispatcherImpl) dispatchNextWG() (madeProgress bool) {
	if !d.state().Current.Valid {
		if !d.alg.HasNext() {
			return false
		}
		d.state().Current = saveLocation(d.alg.Next())
		if !d.state().Current.Valid {
			return false
		}
	}

	port := d.getDispatchingPort()
	if !port.CanSend() {
		return false
	}

	req := messaging.Msg{ID: d.cp.NewID(),
		Src: port.AsRemote(),
		Dst: d.state().Current.CU, Payload: protocol.MapWGReq{WorkGroup: d.state().Current.WG,
			PID:        d.state().Dispatching.Payload.(protocol.LaunchKernelReq).PID,
			Wavefronts: d.state().Current.Locations}}

	d.state().WgTaskIDs[req.ID] = d.cp.TaskTracker().StartReqOut(d, req, d.state().LaunchTaskID)
	port.Send(req)

	d.state().Current.Valid = false
	d.state().NumDispatchedWGs++
	d.state().Inflight[req.ID] = d.state().Current
	d.state().CycleLeft = d.latencyTable[len(d.state().Current.Locations)]

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
	for die := range d.state().DieCyclesLeft {
		if d.state().DieCyclesLeft[die] > 0 {
			d.state().DieCyclesLeft[die]--
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
			PID:        d.state().Dispatching.Payload.(protocol.LaunchKernelReq).PID,
			Wavefronts: loc.locations}}

	d.state().WgTaskIDs[req.ID] = d.cp.TaskTracker().StartReqOut(d, req, d.state().LaunchTaskID)
	port.Send(req)

	d.state().NumDispatchedWGs++
	d.state().Inflight[req.ID] = saveLocation(loc)
	// The die is busy dispatching this WG's wavefronts for
	// wavefrontDispatchCycles*W cycles total. This dispatch consumes the current
	// cycle (the first busy cycle), so the gate holds the remaining W*cycles-1,
	// giving an exact per-die period of W*cycles (not W*cycles+1).
	d.state().DieCyclesLeft[die] = d.wavefrontDispatchCycles*len(loc.locations) - 1

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
