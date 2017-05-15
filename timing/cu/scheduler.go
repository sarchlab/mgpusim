package cu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/mem"
)

// A Scheduler is the central controller of a compute unit.
//
// It is responsible for communicating with the task dispatchers. So it can
// decide whether or not a work-group can be mapped to current CU. It also
// handles the dispatching of wavefronts.
//
// It is also responsible for arbitrating the instruction fetching and issuing.
//
// ToDispatcher <=>  The port conneting the scheduler and the dispatcher
//
// ToSReg <=> The port connecting the scheduler with the scalar register file
//
// ToVRegs <=> The port connecting the scheduler with the vector register files
//
// ToInstMem <=> The port connecting the scheduler with the instruction memory unit
//
// ToDecoders <=> The port connecting the scheduler with the decoders
//
type Scheduler struct {
	*core.ComponentBase

	engine       core.Engine
	wgMapper     WGMapper
	wfDispatcher WfDispatcher
	fetchArbitor WfArbitor
	issueArbitor WfArbitor
	decoder      Decoder // Decoder used to parse fetched instruction

	InstMem          core.Component
	SRegFile         core.Component
	BranchUnit       core.Component
	VectorMemDecoder core.Component
	ScalarDecoder    core.Component
	VectorDecoder    core.Component
	LDSDecoder       core.Component

	WfPools []*WavefrontPool

	used              bool
	Freq              core.Freq
	running           bool
	internalExecuting *Wavefront

	// A set of workgroups running on current CU
	RunningWGs map[*kernels.WorkGroup]*WorkGroup
}

// NewScheduler creates and returns a new Scheduler
func NewScheduler(
	name string,
	engine core.Engine,
	wgMapper WGMapper,
	wfDispatcher WfDispatcher,
	fetchArbitor WfArbitor,
	issueArbitor WfArbitor,
	decoder Decoder,
) *Scheduler {
	s := new(Scheduler)
	s.ComponentBase = core.NewComponentBase(name)

	s.engine = engine
	s.wgMapper = wgMapper
	s.wfDispatcher = wfDispatcher
	s.fetchArbitor = fetchArbitor
	s.issueArbitor = issueArbitor
	s.decoder = decoder

	s.initWfPools([]int{10, 10, 10, 10})
	s.used = false
	s.RunningWGs = make(map[*kernels.WorkGroup]*WorkGroup)

	s.AddPort("ToDispatcher")
	s.AddPort("ToSReg")
	s.AddPort("ToVRegs")
	s.AddPort("ToInstMem")
	s.AddPort("ToDecoders")

	return s
}

func (s *Scheduler) initWfPools(numWfs []int) {
	s.WfPools = make([]*WavefrontPool, 0, len(numWfs))
	for i := 0; i < len(numWfs); i++ {
		s.WfPools = append(s.WfPools, NewWavefrontPool(numWfs[i]))
	}
}

// Recv function process the incoming requests
func (s *Scheduler) Recv(req core.Req) *core.Error {
	s.Lock()
	defer s.Unlock()

	switch req := req.(type) {
	case *timing.MapWGReq:
		return s.processMapWGReq(req)
	case *timing.DispatchWfReq:
		return s.processDispatchWfReq(req)
	case *mem.AccessReq: // Fetch return
		return s.processAccessReq(req)
	default:
		log.Panicf("Unable to process req %s", reflect.TypeOf(req))
	}
	return nil
}

func (s *Scheduler) processMapWGReq(req *timing.MapWGReq) *core.Error {
	s.used = true
	evt := NewMapWGEvent(s.Freq.NextTick(req.RecvTime()), s, req)
	s.engine.Schedule(evt)
	return nil
}

func (s *Scheduler) processDispatchWfReq(
	req *timing.DispatchWfReq,
) *core.Error {
	evt := NewDispatchWfEvent(s.Freq.NextTick(req.RecvTime()), s, req)
	s.engine.Schedule(evt)
	return nil
}

func (s *Scheduler) processAccessReq(req *mem.AccessReq) *core.Error {
	wf := req.Info.(*Wavefront)
	wf.State = WfFetched
	wf.LastFetchTime = req.RecvTime()

	s.decode(req.Buf, wf)

	return nil
}

// This decode is just a virtual step for the simulator. In the simulator,
// there is no difference when the decode actually happen.
func (s *Scheduler) decode(buf []byte, wf *Wavefront) {
	inst, err := s.decoder.Decode(buf)
	if err != nil {
		log.Fatal(err)
	}
	wf.Inst = NewInst(inst)
}

// Handle processes the event that is scheduled on this scheduler
func (s *Scheduler) Handle(evt core.Event) error {
	s.Lock()
	defer s.Unlock()
	s.InvokeHook(evt, core.BeforeEvent)
	defer s.InvokeHook(evt, core.AfterEvent)

	switch evt := evt.(type) {
	case *MapWGEvent:
		return s.handleMapWGEvent(evt)
	case *DispatchWfEvent:
		return s.handleDispatchWfEvent(evt)
	case *core.TickEvent:
		return s.handleTickEvent(evt)
	case *WfCompleteEvent:
		return s.handleWfCompleteEvent(evt)
	default:
		log.Panicf("Cannot handle event type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (s *Scheduler) handleMapWGEvent(evt *MapWGEvent) error {
	req := evt.Req

	ok := s.wgMapper.MapWG(req)

	if ok {
		managedWG := NewWorkGroup(req.WG, req)
		s.RunningWGs[req.WG] = managedWG
	}

	req.Ok = ok
	req.SwapSrcAndDst()
	req.SetSendTime(evt.Time())
	s.GetConnection("ToDispatcher").Send(req)

	return nil
}

func (s *Scheduler) handleDispatchWfEvent(evt *DispatchWfEvent) error {
	done, wf := s.wfDispatcher.DispatchWf(evt)
	if !done {
		evt.SetTime(s.Freq.NextTick(evt.Time()))
		s.engine.Schedule(evt)
	} else {
		wg := s.RunningWGs[evt.Req.Wf.WG]
		wg.Wfs = append(wg.Wfs, wf)

		s.tryScheduleTick(s.Freq.NextTick(evt.Time()))

		// This is temporary code, to be removed later
		// wfCompleteEvent := NewWfCompleteEvent(
		// 	s.Freq.NCyclesLater(3000, evt.Time()), s, wf)
		// s.engine.Schedule(wfCompleteEvent)
	}

	return nil
}

func (s *Scheduler) tryScheduleTick(t core.VTimeInSec) {
	if !s.running {
		s.scheduleTick(t)
	}
}

func (s *Scheduler) scheduleTick(t core.VTimeInSec) {
	evt := core.NewTickEvent(t, s)
	s.engine.Schedule(evt)
}

func (s *Scheduler) handleTickEvent(evt *core.TickEvent) error {
	s.fetch(evt.Time())
	s.issue(evt.Time())
	return nil
}

func (s *Scheduler) fetch(now core.VTimeInSec) {
	wfs := s.fetchArbitor.Arbitrate(s.WfPools)

	if len(wfs) > 0 {
		wf := wfs[0]
		req := mem.NewAccessReq()
		req.Address = wf.PC
		req.Type = mem.Read
		req.ByteSize = 8
		req.SetDst(s.InstMem)
		req.SetSrc(s)
		req.SetSendTime(now)
		req.Info = wf

		err := s.GetConnection("ToInstMem").Send(req)
		if err != nil && !err.Recoverable {
			log.Fatal(err)
		} else if err != nil {
			// Do not do anything
		} else {
			wf.State = WfFetching
		}
	}
}

func (s *Scheduler) issue(now core.VTimeInSec) {
	wfs := s.issueArbitor.Arbitrate(s.WfPools)
	for _, wf := range wfs {
		if wf.Inst.ExeUnit == insts.ExeUnitSpecial {
			s.issueToInternal(wf)
			continue
		}

		req := NewIssueInstReq(s, s.getUnitToIssueTo(wf.Inst.ExeUnit), now,
			s, wf)
		err := s.GetConnection("ToDecoders").Send(req)
		if err != nil && !err.Recoverable {
			log.Panic(err)
		} else if err != nil {
			wf.State = WfFetched
		} else {
			wf.State = WfRunning
		}
	}
}

func (s *Scheduler) issueToInternal(wf *Wavefront) {
	if s.internalExecuting == nil {
		s.internalExecuting = wf
		wf.State = WfRunning
	} else {
		wf.State = WfFetched
	}

}

func (s *Scheduler) getUnitToIssueTo(u insts.ExeUnit) core.Component {
	switch u {
	case insts.ExeUnitBranch:
		return s.BranchUnit
	case insts.ExeUnitLDS:
		return s.LDSDecoder
	case insts.ExeUnitVALU:
		return s.VectorDecoder
	case insts.ExeUnitVMem:
		return s.VectorMemDecoder
	case insts.ExeUnitScalar:
		return s.ScalarDecoder
	default:
		log.Panic("not sure where to dispatch instrcution")
	}
	return nil
}

func (s *Scheduler) handleWfCompleteEvent(evt *WfCompleteEvent) error {
	wf := evt.Wf
	wg := s.RunningWGs[wf.WG]
	wf.State = WfCompleted

	if s.isAllWfInWGCompleted(wg) {
		ok := s.sendWGCompletionMessage(evt, wg)
		if ok {
			s.clearWGResource(wg)
		}
	}

	return nil
}

func (s *Scheduler) isAllWfInWGCompleted(wg *WorkGroup) bool {
	for _, wf := range wg.Wfs {
		if wf.State != WfCompleted {
			return false
		}
	}
	return true
}

func (s *Scheduler) sendWGCompletionMessage(evt *WfCompleteEvent, wg *WorkGroup) bool {
	mapReq := wg.MapReq
	dispatcher := mapReq.Dst() // This is dst since the mapReq has been sent back already
	now := evt.Time()
	mesg := timing.NewWGFinishMesg(s, dispatcher, now, wg.WorkGroup)
	mesg.CUID = mapReq.CUID

	err := s.GetConnection("ToDispatcher").Send(mesg)
	if err != nil {
		if !err.Recoverable {
			log.Fatal(err)
		} else {
			evt.SetTime(s.Freq.NoEarlierThan(err.EarliestRetry))
			s.engine.Schedule(evt)
			return false
		}
	}
	return true
}

func (s *Scheduler) clearWGResource(wg *WorkGroup) {
	s.wgMapper.UnmapWG(wg)
	for _, wf := range wg.Wfs {
		wfPool := s.WfPools[wf.SIMDID]
		wfPool.RemoveWf(wf)
	}
}

func (s *Scheduler) writeReg(
	wf *Wavefront,
	reg *insts.Reg,
	data []byte,
	now core.VTimeInSec,
) {
	if reg.IsSReg() {
		req := NewWriteRegReq(now, reg, wf.SRegOffset, data)
		req.SetSrc(s)
		req.SetDst(s.SRegFile)
		s.GetConnection("ToSReg").Send(req)
	} else {
		req := NewWriteRegReq(now, reg, wf.VRegOffset, data)
		req.SetSrc(s)
		req.SetDst(s.WfPools[wf.SIMDID].VRegFile)
		s.GetConnection("ToVRegs").Send(req)
	}
}

// A WfCompleteEvent marks the competion of a wavefront
type WfCompleteEvent struct {
	*core.EventBase
	Wf *Wavefront
}

// NewWfCompleteEvent returns a newly constructed WfCompleteEvent
func NewWfCompleteEvent(time core.VTimeInSec, handler core.Handler,
	wf *Wavefront,
) *WfCompleteEvent {
	evt := new(WfCompleteEvent)
	evt.EventBase = core.NewEventBase(time, handler)
	evt.Wf = wf
	return evt
}
