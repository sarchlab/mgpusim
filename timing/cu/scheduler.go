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
	Freq    core.Freq

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
	default:
		log.Panicf("Unable to process req %s", reflect.TypeOf(req))
	}

	return nil
}

func (s *Scheduler) processMapWGReq(req *timing.MapWGReq) *core.Error {
	evt := NewMapWGEvent(s, req.RecvTime(), req)
	s.engine.Schedule(evt)
	return nil
}

// Handle processes the event that is scheduled on this scheduler
func (s *Scheduler) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *MapWGEvent:
		return s.handleMapWGEvent(evt)
	default:
		log.Panicf("Cannot handle event type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (s *Scheduler) handleMapWGEvent(evt *MapWGEvent) error {
	req := evt.Req
	req.SwapSrcAndDst()

	req.Ok = true

	s.GetConnection("ToDispatcher").Send(req)
	return nil
}

// FetchInfo keeps record of the information of a fetch action
type FetchInfo struct {
	Buf []byte
	Wf  *Wavefront
}

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront

	PC uint64
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
func NewMapWGEvent(handler core.Handler,
	time core.VTimeInSec,
	req *timing.MapWGReq,
) *MapWGEvent {
	e := new(MapWGEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.Req = req
	return e
}
