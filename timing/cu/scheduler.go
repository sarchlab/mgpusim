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

	engine   core.Engine
	wgMapper *WGMapperImpl

	WfPools []*WavefrontPool

	used    bool
	Freq    core.Freq
	Running bool

	MappedWGs []*timing.MapWGReq
}

// NewScheduler creates and returns a new Scheduler
func NewScheduler(name string, engine core.Engine, wgMapper *WGMapperImpl) *Scheduler {
	s := new(Scheduler)
	s.engine = engine
	s.BasicComponent = core.NewBasicComponent(name)

	s.initWfPools([]int{10, 10, 10, 10})
	s.used = false

	s.wgMapper = wgMapper

	s.AddPort("ToDispatcher")
	return s
}

func (s *Scheduler) initWfPools(numWfs []int) {
	s.WfPools = make([]*WavefrontPool, 0, len(numWfs))
	for i := 0; i < len(numWfs); i++ {
		s.WfPools = append(s.WfPools, NewWavefrontPool(numWfs[i]))
	}
}

// SetWfPoolSize changes the number of wavefront that the scheduler can handle
// The first argument is the number of wavefront pools, which should always
// match the number of SIMDs that the comput unit has. The second argument is
// a slice indicating the number of wavefronts that each wavefront pool can
// hold. This function must be called before the scheduler has been used,
// otherwise it will panic.
func (s *Scheduler) SetWfPoolSize(numWfPool int, numWfs []int) {
	if s.used {
		log.Panic("Scheduler cannot resize after mapped with a work-group")
	}

	// s.initWfPools(numWfPool, numWfs)
	// s.initVGPRInfo(append(s.VGprCount, s.VGprCount[0]))
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
		// return s.wgMapper.handleMapWGEvent(evt)
	case *DispatchWfEvent:
		return s.handleDispatchWfEvent(evt)
	default:
		log.Panicf("Cannot handle event type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (s *Scheduler) handleDispatchWfEvent(evt *DispatchWfEvent) error {
	req := evt.Req
	wf := req.Wf
	info := req.Info

	wfPool := s.WfPools[info.SIMDID]
	managedWf := new(Wavefront)
	managedWf.Wavefront = wf
	wfPool.Wfs = append(wfPool.Wfs, managedWf)

	s.initWfRegs(managedWf, req)

	if !s.Running {
		s.Running = true
		evt := NewScheduleEvent(s, s.Freq.NextTick(evt.Time()))
		s.engine.Schedule(evt)
	}

	return nil
}

func (s *Scheduler) allocateWfRegs(wf *Wavefront, req *timing.DispatchWfReq) {
}

func (s *Scheduler) initWfRegs(wf *Wavefront, req *timing.DispatchWfReq) {
	wf.PC = req.EntryPoint
}

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront

	PC          uint64
	FetchBuffer []byte
	SRegOffset  int
	VRegOffset  int
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
