package cu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// SIMDUnit is a unit that can execute vector instructions
//
// FromDecoder <=>
//
// ToScheduler <=>
//
// ToVReg <=>
//
// ToSReg <=>
type SIMDUnit struct {
	*core.ComponentBase

	engine    core.Engine
	Freq      core.Freq
	scheduler core.Component
	running   bool

	IntALUWidth    int
	DoubleALUWidth int
	SingleALUWidth int

	VRegFile *RegCtrl
	SRegFile *RegCtrl

	readWaiting *Wavefront // A buffer that the instruction is issued, but the read stage is not available yet
	reading     *Wavefront
	executing   *Wavefront
	writing     *Wavefront
}

// NewSIMDUnit returns a newly created SIMDUnit
func NewSIMDUnit(name string, engine core.Engine, scheduler core.Component) *SIMDUnit {
	u := new(SIMDUnit)
	u.ComponentBase = core.NewComponentBase(name)

	u.engine = engine
	u.scheduler = scheduler

	u.IntALUWidth = 16
	u.SingleALUWidth = 16
	u.DoubleALUWidth = 2

	u.AddPort("FromDecoder")
	u.AddPort("ToScheduler")
	u.AddPort("ToVReg")
	u.AddPort("ToSReg")

	return u
}

// Recv defines the how the SIMDUnit process incomming requests
func (u *SIMDUnit) Recv(req core.Req) *core.Error {
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

func (u *SIMDUnit) processIssueInstReq(req *IssueInstReq) *core.Error {
	if u.readWaiting != nil {
		return core.NewError("unit busy", true, u.Freq.NextTick(req.RecvTime()))
	}

	u.readWaiting = req.Wf
	u.tryStartTick(req.RecvTime())
	return nil
}

// Handle defines how the SIMDUnit handles events
func (u *SIMDUnit) Handle(evt core.Event) error {
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

func (u *SIMDUnit) handleTickEvent(evt *core.TickEvent) error {
	u.doWrite(evt.Time())
	u.doExec(evt.Time())
	u.doRead(evt.Time())
	u.tryStartNewInst(evt.Time())

	u.continueTick(u.Freq.NextTick(evt.Time()))

	return nil
}

func (u *SIMDUnit) doWrite(now core.VTimeInSec) {
	if u.writing != nil {
		req := NewInstCompletionReq(u, u.scheduler, now, u.writing)
		err := u.GetConnection("ToScheduler").Send(req)
		if err == nil {
			u.InvokeHook(u.writing, u, core.Any, &InstHookInfo{now, "WriteDone"})
			u.writing = nil
		}
	}
}

func (u *SIMDUnit) doExec(now core.VTimeInSec) {
	if u.executing != nil {
		if u.writing == nil {
			u.executing.CompletedLanes += u.SingleALUWidth
			if u.executing.CompletedLanes == 64 {
				u.InvokeHook(u.executing, u, core.Any, &InstHookInfo{now, "ExecEnd"})
				u.InvokeHook(u.executing, u, core.Any, &InstHookInfo{now, "WriteStart"})
				u.writing = u.executing
				u.executing = nil
			}
		}
	}
}

func (u *SIMDUnit) doRead(now core.VTimeInSec) {
	if u.reading != nil {
		if u.executing == nil {
			u.InvokeHook(u.reading, u, core.Any, &InstHookInfo{now, "ReadDone"})
			u.InvokeHook(u.reading, u, core.Any, &InstHookInfo{now, "ExecStart"})
			u.executing = u.reading
			u.reading = nil
		}
	}
}

func (u *SIMDUnit) tryStartNewInst(now core.VTimeInSec) {
	if u.reading == nil && u.readWaiting != nil {
		u.InvokeHook(u.readWaiting, u, core.Any, &InstHookInfo{now, "ReadStart"})
		u.reading = u.readWaiting
		u.readWaiting = nil
		u.reading.CompletedLanes = 0
	}
}

func (u *SIMDUnit) tryStartTick(now core.VTimeInSec) {
	if !u.running {
		u.scheduleTick(now)
	}
}

func (u *SIMDUnit) continueTick(now core.VTimeInSec) {
	if u.reading == nil &&
		u.executing == nil &&
		u.writing == nil {
		u.running = false
	}

	if u.running {
		u.scheduleTick(now)
	}
}

func (u *SIMDUnit) scheduleTick(now core.VTimeInSec) {
	evt := core.NewTickEvent(now, u)
	u.engine.Schedule(evt)
	u.running = true
}
