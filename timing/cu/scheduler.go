package cu

import (
	"log"
	"reflect"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
)

// A Scheduler is responsible for determine which wavefront can fetch, decode,
// and issue
//
//     ToDispatcher <=>  The port conneting the scheduler and the dispatcher
//     ToSReg <=> The port connecting the scheduler with the scalar register
// 				  file
//     ToVRegs <=> The port connecting ithe scheduler with the vector register
//                files
type Scheduler struct {
	*core.BasicComponent
	sync.Mutex

	engine       core.Engine
	wgMapper     WGMapper
	wfDispatcher WfDispatcher
	SRegFile     core.Component

	WfPools []*WavefrontPool

	used    bool
	Freq    core.Freq
	Running bool

	// A set of workgroups running on current CU
	RunningWGs map[*kernels.WorkGroup]*WorkGroup
}

// NewScheduler creates and returns a new Scheduler
func NewScheduler(
	name string,
	engine core.Engine,
	wgMapper WGMapper,
	wfDispatcher WfDispatcher,
) *Scheduler {
	s := new(Scheduler)
	s.BasicComponent = core.NewBasicComponent(name)

	s.engine = engine
	s.wgMapper = wgMapper
	s.wfDispatcher = wfDispatcher

	s.initWfPools([]int{10, 10, 10, 10})
	s.used = false
	s.RunningWGs = make(map[*kernels.WorkGroup]*WorkGroup)

	s.AddPort("ToDispatcher")
	s.AddPort("ToSReg")
	s.AddPort("ToVRegs")
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

// Handle processes the event that is scheduled on this scheduler
func (s *Scheduler) Handle(evt core.Event) error {
	s.InvokeHook(evt, core.BeforeEvent)
	defer s.InvokeHook(evt, core.AfterEvent)

	switch evt := evt.(type) {
	case *MapWGEvent:
		return s.handleMapWGEvent(evt)
	case *DispatchWfEvent:
		return s.handleDispatchWfEvent(evt)
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

		// This is temporary code, to be removed later
		wfCompleteEvent := NewWfCompleteEvent(
			s.Freq.NCyclesLater(3000, evt.Time()), s, wf)
		s.engine.Schedule(wfCompleteEvent)
	}

	return nil
}

func (s *Scheduler) handleWfCompleteEvent(evt *WfCompleteEvent) error {
	wf := evt.Wf
	wg := s.RunningWGs[wf.WG]
	wf.Status = Completed

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
		// FIXME, is there data race here, or only scheduler can change
		// wavefront status?
		if wf.Status != Completed {
			return false
		}
	}
	return true
}

func (s *Scheduler) sendWGCompletionMessage(evt *WfCompleteEvent, wg *WorkGroup) bool {
	mapReq := wg.MapReq
	dispatcher := mapReq.Dst() // This is dst since the mapReq has been sent back already
	now := evt.Time()
	mesg := timing.NewWGFinishMesg(s, dispatcher, now, wg.WorkGroup,
		mapReq.KernelStatus)
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

// ScheduleEvent requires the scheduler to schedule for the next cycle
type ScheduleEvent struct {
	*core.BasicEvent
}

// NewScheduleEvent returns a newly created ScheduleEvent
func NewScheduleEvent(
	time core.VTimeInSec,
	handler core.Handler,
) *ScheduleEvent {
	e := new(ScheduleEvent)
	e.BasicEvent = core.NewBasicEvent(time, handler)
	return e
}

// A WfCompleteEvent marks the competion of a wavefront
type WfCompleteEvent struct {
	*core.BasicEvent
	Wf *Wavefront
}

// NewWfCompleteEvent returns a newly constructed WfCompleteEvent
func NewWfCompleteEvent(time core.VTimeInSec, handler core.Handler,
	wf *Wavefront,
) *WfCompleteEvent {
	evt := new(WfCompleteEvent)
	evt.BasicEvent = core.NewBasicEvent(time, handler)
	evt.Wf = wf
	return evt
}
