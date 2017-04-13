package cu

import (
	"log"
	"reflect"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
)

// A Scheduler is responsible for determine which wavefront can fetch, decode,
// and issue
//
//    <=> ToDispatcher The port conneting the scheduler and the dispatcher
//
type Scheduler struct {
	*core.BasicComponent
	sync.Mutex

	engine core.Engine

	WfPools []*WavefrontPool

	Freq            core.Freq
	NumWfsCanHandle int
	Running         bool
	NextWfPool      int

	MappedWGs []*timing.MapWGReq
}

// NewScheduler creates and returns a new Scheduler
func NewScheduler(name string, engine core.Engine) *Scheduler {
	s := new(Scheduler)
	s.engine = engine
	s.BasicComponent = core.NewBasicComponent(name)
	s.WfPools = make([]*WavefrontPool, 0, 4)
	for i := 0; i < 4; i++ {
		s.WfPools = append(s.WfPools, NewWavefrontPool())
	}
	s.NextWfPool = 0

	s.NumWfsCanHandle = 40

	s.AddPort("ToDispatcher")
	return s
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
	evt := NewMapWGEvent(s, s.Freq.NextTick(req.RecvTime()), req)
	s.engine.Schedule(evt)
	return nil
}

func (s *Scheduler) processDispatchWfReq(
	req *timing.DispatchWfReq,
) *core.Error {
	evt := NewDispatchWfEvent(s, s.Freq.NextTick(req.RecvTime()), req)
	s.engine.Schedule(evt)
	return nil
}

// Handle processes the event that is scheduled on this scheduler
func (s *Scheduler) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *MapWGEvent:
		return s.handleMapWGEvent(evt)
	case *DispatchWfEvent:
		return s.handleDispatchWfEvent(evt)
	default:
		log.Panicf("Cannot handle event type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (s *Scheduler) handleMapWGEvent(evt *MapWGEvent) error {
	req := evt.Req
	req.SwapSrcAndDst()
	req.SetSendTime(evt.Time())

	if s.NumWfsCanHandle < len(req.WG.Wavefronts) {
	} else {
		req.Ok = true
		s.NumWfsCanHandle -= len(req.WG.Wavefronts)
	}

	s.GetConnection("ToDispatcher").Send(req)
	return nil
}

func (s *Scheduler) handleDispatchWfEvent(evt *DispatchWfEvent) error {
	req := evt.Req
	wf := req.Wf

	wfPool := s.WfPools[s.NextWfPool]
	managedWf := new(Wavefront)
	managedWf.Wavefront = wf
	wfPool.Wfs = append(wfPool.Wfs, managedWf)

	s.NextWfPool++
	if s.NextWfPool >= len(s.WfPools) {
		s.NextWfPool = 0
	}

	if !s.Running {
		s.Running = true
		evt := NewScheduleEvent(s, s.Freq.NextTick(evt.Time()))
		s.engine.Schedule(evt)
	}

	return nil
}

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront

	PC          uint64
	FetchBuffer []byte
}

// A FetchArbitrator can decide which wavefront in a scheduler can fetch
// instructions
type FetchArbitrator interface {
}

// An IssueArbitrator decides which wavefront can issue instruction
type IssueArbitrator interface {
}

// MapWGEvent requres the Scheduler to reserve space for a workgroup.
// The workgroup will not run immediately. The dispatcher will wait for the
// scheduler to dispatch wavefronts to it.
type MapWGEvent struct {
	*core.BasicEvent

	Req *timing.MapWGReq
}

// NewMapWGEvent creates a new MapWGEvent
func NewMapWGEvent(
	handler core.Handler,
	time core.VTimeInSec,
	req *timing.MapWGReq,
) *MapWGEvent {
	e := new(MapWGEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.SetHandler(handler)
	e.SetTime(time)
	e.Req = req
	return e
}

// DispatchWfEvent requires the scheduler shart to schedule for the event.
type DispatchWfEvent struct {
	*core.BasicEvent

	Req *timing.DispatchWfReq
}

// NewDispatchWfEvent returns a newly created DispatchWfEvent
func NewDispatchWfEvent(
	handler core.Handler,
	time core.VTimeInSec,
	req *timing.DispatchWfReq,
) *DispatchWfEvent {
	e := new(DispatchWfEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.SetHandler(handler)
	e.SetTime(time)
	e.Req = req
	return e
}

// ScheduleEvent requires the scheduler to schedule for the next cycle
type ScheduleEvent struct {
	*core.BasicEvent
}

// NewScheduleEvent returns a newly created ScheduleEvent
func NewScheduleEvent(
	handler core.Handler,
	time core.VTimeInSec,
) *ScheduleEvent {
	e := new(ScheduleEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.SetHandler(handler)
	e.SetTime(time)
	return e
}
