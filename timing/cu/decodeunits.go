package cu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// DecodeCompletionEvent is an event that marks the completion of decoding
type DecodeCompletionEvent struct {
	*core.EventBase
	*IssueInstReq
}

// NewDecodeCompletionEvent creates a new DecodeCompletionEvent
func NewDecodeCompletionEvent(
	time core.VTimeInSec,
	handler core.Handler,
	req *IssueInstReq,
) *DecodeCompletionEvent {
	e := new(DecodeCompletionEvent)
	e.EventBase = core.NewEventBase(time, handler)
	e.IssueInstReq = req
	return e
}

// SimpleDecodeUnit defines a decode unit that only has one output unit.
//
// FromScheduler <=> The port that receives command from scheduler
//
// ToExecUnit <=> The port to the execution unit
type SimpleDecodeUnit struct {
	*core.ComponentBase

	Freq    core.Freq
	Latency int
	engine  core.Engine

	ExecUnit         core.Component
	available        bool
	nextPossibleTime core.VTimeInSec
}

// NewSimpleDecodeUnit returns a newly constructed SimpleDecodeUnit
func NewSimpleDecodeUnit(name string, engine core.Engine) *SimpleDecodeUnit {
	u := new(SimpleDecodeUnit)
	u.ComponentBase = core.NewComponentBase(name)
	u.engine = engine
	u.available = true

	u.AddPort("FromScheduler")
	u.AddPort("ToExecUnit")

	return u
}

// Recv processes the imcoming requests
func (u *SimpleDecodeUnit) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *IssueInstReq:
		return u.processIssueInstReq(req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
	return nil
}

func (u *SimpleDecodeUnit) processIssueInstReq(req *IssueInstReq) *core.Error {
	if !u.available {
		return core.NewError("busy", true, u.nextPossibleTime)
	}
	completionTime := u.Freq.NCyclesLater(u.Latency, req.RecvTime())
	evt := NewDecodeCompletionEvent(completionTime, u, req)
	u.engine.Schedule(evt)
	u.available = false
	u.nextPossibleTime = u.Freq.NextTick(completionTime)
	return nil
}

// Handle defines what happens on event triggered on this component
func (u *SimpleDecodeUnit) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *DecodeCompletionEvent:
		return u.handleDecodeCompletionEvent(evt)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (u *SimpleDecodeUnit) handleDecodeCompletionEvent(
	evt *DecodeCompletionEvent,
) error {
	req := evt.IssueInstReq
	req.SetSrc(u)
	req.SetDst(u.ExecUnit)
	req.SetSendTime(evt.Time())

	err := u.GetConnection("ToExecUnit").Send(req)
	if err != nil {
		if !err.Recoverable {
			log.Fatal(err)
		} else {
			u.nextPossibleTime = u.Freq.NextTick(err.EarliestRetry)
			evt.SetTime(err.EarliestRetry)
			u.engine.Schedule(evt)
		}
	} else {
		u.available = true
	}

	return nil
}
