package timing

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
)

// LDSUnit is the execution unit that is responsible for executing the
// local data share instuctions
//
// ToScheduler <=>
//
// FromDecoder <=>
type LDSUnit struct {
	*core.ComponentBase

	Freq util.Freq

	engine    core.Engine
	scheduler core.Component
	running   bool

	reading   *Wavefront
	executing *Wavefront
	writing   *Wavefront
}

// NewLDSUnit creates and retuns a new LDSUnit
func NewLDSUnit(name string, engine core.Engine, scheduler core.Component) *LDSUnit {
	u := new(LDSUnit)
	u.ComponentBase = core.NewComponentBase(name)

	u.engine = engine
	u.scheduler = scheduler

	u.AddPort("ToScheduler")
	u.AddPort("FromDecoder")
	return u
}

// Recv defines the how the LDSUnit process incomming requests
func (u *LDSUnit) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *IssueInstReq:
		return u.processIssueInstReq(req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
	return nil
}

func (u *LDSUnit) processIssueInstReq(req *IssueInstReq) *core.Error {
	if u.reading != nil {
		return core.NewError("unit busy", true, u.Freq.NextTick(req.RecvTime()))
	}

	u.reading = req.Wf
	u.tryStartTick(req.RecvTime())
	return nil
}

// Handle defines how the LDSUnit handles events
func (u *LDSUnit) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *core.TickEvent:
		return u.handleTickEvent(evt)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (u *LDSUnit) handleTickEvent(evt *core.TickEvent) error {
	u.doWrite(evt.Time())
	u.doExec(evt.Time())
	u.doRead(evt.Time())

	u.continueTick(evt.Time())

	return nil
}

func (u *LDSUnit) doWrite(now core.VTimeInSec) {
	if u.writing != nil {
		req := NewInstCompletionReq(u, u.scheduler, now, u.writing)
		err := u.GetConnection("ToScheduler").Send(req)
		if err == nil {
			u.writing = nil
		}
	}
}

func (u *LDSUnit) doExec(now core.VTimeInSec) {
	if u.executing != nil {
		if u.writing == nil {
			u.writing = u.executing
			u.executing = nil
		}
	}
}

func (u *LDSUnit) doRead(now core.VTimeInSec) {
	if u.reading != nil {
		if u.executing == nil {
			u.executing = u.reading
			u.reading = nil
		}
	}
}

func (u *LDSUnit) tryStartTick(now core.VTimeInSec) {
	if !u.running {
		u.scheduleTick(now)
	}
}

func (u *LDSUnit) continueTick(now core.VTimeInSec) {
	if u.reading == nil &&
		u.executing == nil &&
		u.writing == nil {
		u.running = false
	}

	if u.running {
		u.scheduleTick(now)
	}
}

func (u *LDSUnit) scheduleTick(now core.VTimeInSec) {
	evt := core.NewTickEvent(now, u)
	u.engine.Schedule(evt)
	u.running = true
}
