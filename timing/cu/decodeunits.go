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

	ExecUnit core.Component
	toDecode *Wavefront
	decoded  *Wavefront
}

// NewSimpleDecodeUnit returns a newly constructed SimpleDecodeUnit
func NewSimpleDecodeUnit(name string, engine core.Engine) *SimpleDecodeUnit {
	u := new(SimpleDecodeUnit)
	u.ComponentBase = core.NewComponentBase(name)
	u.engine = engine

	u.AddPort("FromScheduler")
	u.AddPort("ToExecUnit")

	return u
}

// Recv processes the incoming requests
func (u *SimpleDecodeUnit) Recv(req core.Req) *core.Error {
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

func (u *SimpleDecodeUnit) processIssueInstReq(req *IssueInstReq) *core.Error {
	if u.toDecode != nil {
		return core.NewError("busy", true, u.Freq.NextTick(req.RecvTime()))
	}

	u.toDecode = req.Wf
	u.InvokeHook(req.Wf, u, core.Any, &InstHookInfo{req.RecvTime(), "DecodeStart"})
	completionTime := u.Freq.NCyclesLater(u.Latency-1, req.RecvTime())
	evt := NewDecodeCompletionEvent(completionTime, u, req)
	u.engine.Schedule(evt)
	return nil
}

// Handle defines what happens on event triggered on this component
func (u *SimpleDecodeUnit) Handle(evt core.Event) error {
	u.Lock()
	defer u.Unlock()

	switch evt := evt.(type) {
	case *DecodeCompletionEvent:
		return u.handleDecodeCompletionEvent(evt)
	case *core.DeferredSend:
		return u.handleDeferredSend(evt)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (u *SimpleDecodeUnit) handleDecodeCompletionEvent(
	evt *DecodeCompletionEvent,
) error {
	// Output is not cleared yet
	if u.decoded != nil {
		evt.SetTime(u.Freq.NextTick(evt.Time()))
		u.engine.Schedule(evt)
		return nil
	}

	// Schedule send
	req := evt.IssueInstReq
	req.SetSrc(u)
	req.SetDst(u.ExecUnit)
	req.SetSendTime(u.Freq.HalfTick(evt.Time()))
	u.InvokeHook(req.Wf, u, core.Any,
		&InstHookInfo{evt.Time(), "DecodeDone"})

	u.decoded = u.toDecode
	u.toDecode = nil

	deferredSend := core.NewDeferredSend(req)
	u.engine.Schedule(deferredSend)

	return nil
}

func (u *SimpleDecodeUnit) handleDeferredSend(evt *core.DeferredSend) error {
	req := evt.Req
	err := u.GetConnection("ToExecUnit").Send(req)
	if err != nil {
		if !err.Recoverable {
			log.Fatal(err)
		} else {
			evt.SetTime(u.Freq.HalfTick(err.EarliestRetry))
			u.engine.Schedule(evt)
		}
	} else {
		u.decoded = nil
	}
	return nil
}

// VectorDecodeUnit defines a decode unit that is for the vector ALU ]
// instructions. It can dispatch the instruction to different SIMD units.
//
// FromScheduler <=> The port that receives command from scheduler
//
// ToExecUnit <=> The port to the execution unit
type VectorDecodeUnit struct {
	*core.ComponentBase

	Freq    core.Freq
	Latency int
	engine  core.Engine

	SIMDUnits []core.Component
	toDecode  *Wavefront
	decoded   *Wavefront
}

// NewVectorDecodeUnit returns a newly constructed SimpleDecodeUnit
func NewVectorDecodeUnit(name string, engine core.Engine) *VectorDecodeUnit {
	u := new(VectorDecodeUnit)
	u.ComponentBase = core.NewComponentBase(name)
	u.engine = engine
	u.SIMDUnits = make([]core.Component, 0)

	u.AddPort("FromScheduler")
	u.AddPort("ToExecUnit")

	return u
}

// Recv processes the incoming requests
func (u *VectorDecodeUnit) Recv(req core.Req) *core.Error {
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

func (u *VectorDecodeUnit) processIssueInstReq(req *IssueInstReq) *core.Error {
	if u.toDecode != nil {
		return core.NewError("busy", true, u.Freq.NextTick(req.RecvTime()))
	}

	u.InvokeHook(req.Wf, u, core.Any, &InstHookInfo{req.RecvTime(), "DecodeStart"})
	completionTime := u.Freq.NCyclesLater(u.Latency-1, req.RecvTime())
	evt := NewDecodeCompletionEvent(completionTime, u, req)
	u.engine.Schedule(evt)
	return nil
}

// Handle defines what happens on event triggered on this component
func (u *VectorDecodeUnit) Handle(evt core.Event) error {
	u.Lock()
	defer u.Unlock()

	switch evt := evt.(type) {
	case *DecodeCompletionEvent:
		return u.handleDecodeCompletionEvent(evt)
	case *core.DeferredSend:
		return u.handleDeferredSend(evt)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (u *VectorDecodeUnit) handleDecodeCompletionEvent(
	evt *DecodeCompletionEvent,
) error {
	wf := evt.Wf
	req := evt.IssueInstReq
	req.SetSrc(u)
	req.SetDst(u.SIMDUnits[wf.SIMDID])
	req.SetSendTime(u.Freq.HalfTick(evt.Time()))
	u.InvokeHook(wf, u, core.Any, &InstHookInfo{evt.Time(), "DecodeDone"})

	if u.decoded != nil {
		evt.SetTime(u.Freq.NextTick(evt.Time()))
		u.engine.Schedule(evt)
		return nil
	}

	u.decoded = u.toDecode
	u.toDecode = nil

	deferredSend := core.NewDeferredSend(req)
	u.engine.Schedule(deferredSend)

	return nil
}

func (u *VectorDecodeUnit) handleDeferredSend(evt *core.DeferredSend) error {
	req := evt.Req
	err := u.GetConnection("ToExecUnit").Send(req)
	if err != nil {
		if !err.Recoverable {
			log.Fatal(err)
		} else {
			evt.SetTime(u.Freq.HalfTick(err.EarliestRetry))
			u.engine.Schedule(evt)
		}
	} else {
		u.decoded = nil
	}
	return nil
}
