package cu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// ScalarUnit is the execution unit that is responsible for executing the
// local data share instuctions
//
// ToScheduler <=>
//
// FromDecoder <=>
type ScalarUnit struct {
	*core.ComponentBase

	Freq core.Freq

	engine    core.Engine
	scheduler core.Component
	running   bool

	readWaiting *Wavefront
	reading     *Wavefront
	executing   *Wavefront
	writing     *Wavefront
}

// NewScalarUnit creates and retuns a new ScalarUnit
func NewScalarUnit(name string, engine core.Engine, scheduler core.Component) *ScalarUnit {
	u := new(ScalarUnit)
	u.ComponentBase = core.NewComponentBase(name)

	u.engine = engine
	u.scheduler = scheduler

	u.AddPort("ToScheduler")
	u.AddPort("FromDecoder")
	return u
}

// Recv defines the how the ScalarUnit process incomming requests
func (u *ScalarUnit) Recv(req core.Req) *core.Error {
	u.Lock()
	defer u.Unlock()

	switch req := req.(type) {
	case *IssueInstReq:
		return u.processIssueInstReq(req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
	return nil
}

func (u *ScalarUnit) processIssueInstReq(req *IssueInstReq) *core.Error {
	if u.readWaiting != nil {
		return core.NewError("unit busy", true, u.Freq.NextTick(req.RecvTime()))
	}

	u.readWaiting = req.Wf
	u.tryStartTick(req.RecvTime())
	return nil
}

// Handle defines how the ScalarUnit handles events
func (u *ScalarUnit) Handle(evt core.Event) error {
	u.Lock()
	defer u.Unlock()

	switch evt := evt.(type) {
	case *core.TickEvent:
		return u.handleTickEvent(evt)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (u *ScalarUnit) handleTickEvent(evt *core.TickEvent) error {
	u.doWrite(evt.Time())
	u.doExec(evt.Time())
	u.doRead(evt.Time())
	u.tryStartNewInst(evt.Time())

	u.continueTick(u.Freq.NextTick(evt.Time()))

	return nil
}

func (u *ScalarUnit) doWrite(now core.VTimeInSec) {
	if u.writing != nil {
		req := NewInstCompletionReq(u, u.scheduler, now, u.writing)
		err := u.GetConnection("ToScheduler").Send(req)
		if err == nil {
			u.InvokeHook(u.writing, u, core.Any, &InstHookInfo{now, "WriteDone"})
			u.writing = nil
		}
	}
}

func (u *ScalarUnit) doExec(now core.VTimeInSec) {
	if u.executing != nil {
		if u.writing == nil {
			u.InvokeHook(u.executing, u, core.Any, &InstHookInfo{now, "ExecEnd"})
			u.InvokeHook(u.executing, u, core.Any, &InstHookInfo{now, "WriteStart"})
			u.writing = u.executing
			u.executing = nil
		}
	}
}

func (u *ScalarUnit) doRead(now core.VTimeInSec) {
	if u.reading != nil {
		if u.executing == nil {
			u.InvokeHook(u.reading, u, core.Any, &InstHookInfo{now, "ReadEnd"})
			u.InvokeHook(u.reading, u, core.Any, &InstHookInfo{now, "ExecStart"})
			u.executing = u.reading
			u.reading = nil
		}
	}
}

func (u *ScalarUnit) tryStartNewInst(now core.VTimeInSec) {
	if u.reading == nil && u.readWaiting != nil {
		u.InvokeHook(u.readWaiting, u, core.Any, &InstHookInfo{now, "ReadStart"})
		u.reading = u.readWaiting
		u.readWaiting = nil
		u.reading.CompletedLanes = 0
	}
}

func (u *ScalarUnit) tryStartTick(now core.VTimeInSec) {
	if !u.running {
		u.scheduleTick(now)
	}
}

func (u *ScalarUnit) continueTick(now core.VTimeInSec) {
	if u.reading == nil &&
		u.executing == nil &&
		u.writing == nil {
		u.running = false
	}

	if u.running {
		u.scheduleTick(now)
	}
}

func (u *ScalarUnit) scheduleTick(now core.VTimeInSec) {
	evt := core.NewTickEvent(now, u)
	u.engine.Schedule(evt)
	u.running = true
}
