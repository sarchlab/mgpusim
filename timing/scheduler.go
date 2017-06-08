package timing

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
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
// FromExecUnits <=> The port to receive InstCompletionReq from execution units
//
type Scheduler struct {
	*core.ComponentBase

	engine       core.Engine
	wgMapper     WGMapper
	wfDispatcher WfDispatcher
	fetchArbiter WfArbiter
	issueArbiter WfArbiter
	decoder      emu.Decoder // Decoder used to parse fetched instruction

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

	// A set of work-groups running on current CU
	RunningWGs map[*kernels.WorkGroup]*WorkGroup
}

// NewScheduler creates and returns a new Scheduler
func NewScheduler(
	name string,
	engine core.Engine,
	wgMapper WGMapper,
	wfDispatcher WfDispatcher,
	fetchArbitor WfArbiter,
	issueArbitor WfArbiter,
	decoder Decoder,
) *Scheduler {
	s := new(Scheduler)
	s.ComponentBase = core.NewComponentBase(name)

	s.engine = engine
	s.wgMapper = wgMapper
	s.wfDispatcher = wfDispatcher
	s.fetchArbiter = fetchArbitor
	s.issueArbiter = issueArbitor
	s.decoder = decoder

	s.initWfPools([]int{10, 10, 10, 10})
	s.used = false
	s.RunningWGs = make(map[*kernels.WorkGroup]*WorkGroup)

	s.AddPort("ToDispatcher")
	s.AddPort("ToSReg")
	s.AddPort("ToVRegs")
	s.AddPort("ToInstMem")
	s.AddPort("ToDecoders")
	s.AddPort("FromExecUnits")

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
	switch req := req.(type) {
	case *gcn3.MapWGReq:
		return s.processMapWGReq(req)
	case *gcn3.DispatchWfReq:
		return s.processDispatchWfReq(req)
	case *mem.AccessReq: // Fetch return
		return s.processAccessReq(req)
	case *InstCompletionReq: // Issue return
		return s.processInstCompletionReq(req)
	default:
		log.Panicf("Unable to process req %s", reflect.TypeOf(req))
	}
	return nil
}

func (s *Scheduler) processMapWGReq(req *gcn3.MapWGReq) *core.Error {
	s.engine.Schedule(req)
	return nil
}

func (s *Scheduler) processDispatchWfReq(
	req *gcn3.DispatchWfReq,
) *core.Error {
	evt := NewDispatchWfEvent(s.Freq.NextTick(req.RecvTime()), s, req)
	s.engine.Schedule(evt)
	return nil
}

func (s *Scheduler) processAccessReq(req *mem.AccessReq) *core.Error {
	wf := req.Info.(*Wavefront)
	wf.Lock()
	wf.State = WfFetched
	wf.LastFetchTime = req.RecvTime()
	s.decode(req.Buf, wf)
	wf.PC += uint64(wf.Inst.ByteSize)
	wf.Unlock()

	s.InvokeHook(wf, s, core.Any, &InstHookInfo{req.RecvTime(), "FetchDone"})

	return nil
}

// This decode is just a virtual step for the simulator. In the simulator,
// there is no difference when the decode actually happen.
func (s *Scheduler) decode(buf []byte, wf *Wavefront) {
	inst, err := s.decoder.Decode(buf)
	if err != nil {
		log.Fatal(err)
	}
	wf.Inst.Inst = inst
}

func (s *Scheduler) processInstCompletionReq(req *InstCompletionReq) *core.Error {
	wf := req.Wf
	s.InvokeHook(wf, s, core.Any, &InstHookInfo{req.RecvTime(), "Completed"})
	wf.Lock()
	wf.State = WfReady
	wf.Unlock()

	return nil
}

// Handle processes the event that is scheduled on this scheduler
func (s *Scheduler) Handle(evt core.Event) error {
	s.Lock()
	defer s.Unlock()

	s.InvokeHook(evt, s, core.BeforeEvent, nil)
	defer s.InvokeHook(evt, s, core.AfterEvent, nil)

	switch evt := evt.(type) {
	case *gcn3.MapWGReq:
		return s.handleMapWGReq(evt)
	case *DispatchWfEvent:
		return s.handleDispatchWfEvent(evt)
	case *core.TickEvent:
		return s.handleTickEvent(evt)
	case *core.DeferredSend:
		return s.handleDeferredSend(evt)
	case *WfCompleteEvent:
		return s.handleWfCompleteEvent(evt)
	default:
		log.Panicf("Cannot handle event type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (s *Scheduler) handleMapWGReq(req *gcn3.MapWGReq) error {
	s.used = true

	ok := s.wgMapper.MapWG(req)

	if ok {
		managedWG := NewWorkGroup(req.WG, req)
		s.RunningWGs[req.WG] = managedWG
	}

	req.Ok = ok
	req.SwapSrcAndDst()
	req.SetSendTime(req.Time())
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
	s.running = true
}

func (s *Scheduler) handleTickEvent(evt *core.TickEvent) error {
	s.executeInternalInst(evt.Time())
	s.fetch(evt.Time())
	s.issue(evt.Time())

	if s.running {
		s.scheduleTick(s.Freq.NextTick(evt.Time()))
	}

	return nil
}

func (s *Scheduler) fetch(now core.VTimeInSec) {
	wfs := s.fetchArbiter.Arbitrate(s.WfPools)

	if len(wfs) > 0 {
		wf := wfs[0]
		wf.Lock()
		wf.Inst = NewInst(nil)
		wf.Unlock()

		req := mem.NewAccessReq()
		req.Address = wf.PC
		req.Type = mem.Read
		req.ByteSize = 8
		req.SetDst(s.InstMem)
		req.SetSrc(s)
		req.SetSendTime(s.Freq.HalfTick(now))
		req.Info = wf

		deferredSend := core.NewDeferredSend(req)
		s.engine.Schedule(deferredSend)
	}
}

func (s *Scheduler) issue(now core.VTimeInSec) {
	wfs := s.issueArbiter.Arbitrate(s.WfPools)
	for _, wf := range wfs {
		if wf.Inst.ExeUnit == insts.ExeUnitSpecial {
			wf.Lock()
			s.issueToInternal(wf, now)
			wf.Unlock()
			continue
		}

		wf.RLock()
		unit := s.getUnitToIssueTo(wf.Inst.ExeUnit)
		req := NewIssueInstReq(s, unit, s.Freq.HalfTick(now), s, wf)
		wf.RUnlock()

		deferredSend := core.NewDeferredSend(req)
		s.engine.Schedule(deferredSend)
	}
}

func (s *Scheduler) issueToInternal(wf *Wavefront, now core.VTimeInSec) {
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

func (s *Scheduler) executeInternalInst(now core.VTimeInSec) {
	if s.internalExecuting == nil {
		return
	}

	executing := s.internalExecuting

	switch s.internalExecuting.Inst.Opcode {
	case 1: // S_ENDPGM
		s.evalSEndPgm(s.internalExecuting, now)
	default:
		// The program has to make progress
		s.internalExecuting.State = WfReady
		s.internalExecuting = nil
	}

	if s.internalExecuting == nil {
		s.InvokeHook(executing, s, core.Any, &InstHookInfo{now, "Completed"})
	}
}

func (s *Scheduler) evalSEndPgm(wf *Wavefront, now core.VTimeInSec) {
	wfCompleteEvt := NewWfCompleteEvent(s.Freq.NextTick(now), s, wf)
	s.engine.Schedule(wfCompleteEvt)
	s.internalExecuting = nil
}

func (s *Scheduler) handleDeferredSend(evt *core.DeferredSend) error {
	req := evt.Req
	switch req := req.(type) {
	case *IssueInstReq:
		return s.doSendIssueInstReq(req)
	case *mem.AccessReq:
		return s.doSendMemAccessReq(req)
	}
	return nil
}

func (s *Scheduler) doSendIssueInstReq(req *IssueInstReq) error {
	wf := req.Wf
	err := s.GetConnection("ToDecoders").Send(req)
	if err != nil && !err.Recoverable {
		log.Panic(err)
	} else if err != nil {
		wf.Lock()
		wf.State = WfFetched
		wf.Unlock()
	} else {
		s.InvokeHook(wf, s, core.Any, &InstHookInfo{req.SendTime(), "Issue"})
		wf.Lock()
		wf.State = WfRunning
		wf.Unlock()
	}
	return nil
}

func (s *Scheduler) doSendMemAccessReq(req *mem.AccessReq) error {
	wf := req.Info.(*Wavefront)
	err := s.GetConnection("ToInstMem").Send(req)
	if err != nil && !err.Recoverable {
		log.Fatal(err)
	} else if err != nil {
		// Do not do anything
	} else {
		wf.Lock()
		wf.State = WfFetching
		wf.Unlock()
		s.InvokeHook(wf, s, core.Any, &InstHookInfo{req.SendTime(), "FetchStart"})
	}
	return nil
}

func (s *Scheduler) handleWfCompleteEvent(evt *WfCompleteEvent) error {
	wf := evt.Wf
	wg := s.RunningWGs[wf.WG]
	wf.Lock()
	wf.State = WfCompleted
	wf.Unlock()

	if s.isAllWfInWGCompleted(wg) {
		ok := s.sendWGCompletionMessage(evt, wg)
		if ok {
			s.clearWGResource(wg)
			delete(s.RunningWGs, wf.WG)
		}
	}

	if len(s.RunningWGs) == 0 {
		s.running = false
	}

	return nil
}

func (s *Scheduler) isAllWfInWGCompleted(wg *WorkGroup) bool {
	for _, wf := range wg.Wfs {
		wf.RLock()
		if wf.State != WfCompleted {
			wf.RUnlock()
			return false
		}
		wf.RUnlock()
	}
	return true
}

func (s *Scheduler) sendWGCompletionMessage(evt *WfCompleteEvent, wg *WorkGroup) bool {
	mapReq := wg.MapReq
	dispatcher := mapReq.Dst() // This is dst since the mapReq has been sent back already
	now := evt.Time()
	mesg := gcn3.NewWGFinishMesg(s, dispatcher, now, wg.WorkGroup)
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
		// req := NewWriteRegReq(now, reg, wf.SRegOffset, data)
		// req.SetSrc(s)
		// req.SetDst(s.SRegFile)
		// s.GetConnection("ToSReg").Send(req)
	} else {
		req := NewWriteRegReq(now, reg, wf.VRegOffset, data)
		req.SetSrc(s)
		req.SetDst(s.WfPools[wf.SIMDID].VRegFile)
		s.GetConnection("ToVRegs").Send(req)
	}
}

// A WfCompleteEvent marks the completion of a wavefront
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
