package sim

import (
	"log"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/sarchlab/akita/v5/hooking"
)

// A SerialEngine is an Engine that always run events one after another.
type SerialEngine struct {
	hooking.HookableBase

	time           VTimeInSec
	queue          *unsafeEventQueue
	secondaryQueue *unsafeEventQueue

	paused    int32 // atomic: 0 = running, 1 = paused
	pauseMu   sync.Mutex
	pauseCond *sync.Cond

	singleRunLock sync.Mutex

	registry map[string]Handler
}

// NewSerialEngine creates a SerialEngine
func NewSerialEngine() *SerialEngine {
	e := new(SerialEngine)

	e.queue = newUnsafeEventQueue()
	e.secondaryQueue = newUnsafeEventQueue()
	e.registry = make(map[string]Handler)
	e.pauseCond = sync.NewCond(&e.pauseMu)

	return e
}

// RegisterHandler registers a handler with the given name.
func (e *SerialEngine) RegisterHandler(name string, handler Handler) {
	e.registry[name] = handler
}

// Schedule register an event to be happen in the future
func (e *SerialEngine) Schedule(evt Event) {
	if evt.Time() < e.time {
		log.Panic("scheduling an event earlier than current time")
	}

	if evt.IsSecondary() {
		e.secondaryQueue.Push(evt)

		return
	}

	e.queue.Push(evt)
}

// Run processes all the events scheduled in the SerialEngine
func (e *SerialEngine) Run() error {
	e.singleRunLock.Lock()
	defer e.singleRunLock.Unlock()

	hasHooks := e.NumHooks() > 0

	for {
		if e.noMoreEvent() {
			return nil
		}

		// Lightweight pause check: atomic load is ~1ns when not paused
		if atomic.LoadInt32(&e.paused) != 0 {
			e.waitForResume()
		}

		evt := e.nextEvent()

		if evt.Time() < e.time {
			log.Panicf(
				"cannot run event in the past, evt %s @ %d, now %d",
				reflect.TypeOf(evt), evt.Time(), e.time,
			)
		}

		e.time = evt.Time()

		if hasHooks {
			hookCtx := hooking.HookCtx{
				Domain: e,
				Pos:    HookPosBeforeEvent,
				Item:   evt,
			}
			e.InvokeHook(hookCtx)

			handler := e.registry[evt.HandlerID()]
			_ = handler.Handle(evt)

			hookCtx.Pos = HookPosAfterEvent
			e.InvokeHook(hookCtx)
		} else {
			handler := e.registry[evt.HandlerID()]
			_ = handler.Handle(evt)
		}
	}
}

// waitForResume blocks until the engine is unpaused.
func (e *SerialEngine) waitForResume() {
	e.pauseMu.Lock()
	for atomic.LoadInt32(&e.paused) != 0 {
		e.pauseCond.Wait()
	}
	e.pauseMu.Unlock()
}

func (e *SerialEngine) noMoreEvent() bool {
	return e.queue.Len() == 0 && e.secondaryQueue.Len() == 0
}

func (e *SerialEngine) nextEvent() Event {
	if e.queue.Len() == 0 {
		return e.secondaryQueue.Pop()
	}

	if e.secondaryQueue.Len() == 0 {
		return e.queue.Pop()
	}

	primaryEvt := e.queue.Peek()
	secondaryEvt := e.secondaryQueue.Peek()

	if primaryEvt.Time() <= secondaryEvt.Time() {
		e.queue.Pop()
		return primaryEvt
	}

	e.secondaryQueue.Pop()

	return secondaryEvt
}

// Pause prevents the SerialEngine to trigger more events.
func (e *SerialEngine) Pause() {
	e.pauseMu.Lock()
	defer e.pauseMu.Unlock()

	atomic.StoreInt32(&e.paused, 1)
}

// Continue allows the SerialEngine to trigger more events.
func (e *SerialEngine) Continue() {
	e.pauseMu.Lock()
	defer e.pauseMu.Unlock()

	atomic.StoreInt32(&e.paused, 0)
	e.pauseCond.Broadcast()
}

// CurrentTime returns the current time at which the engine is at.
// Specifically, the run time of the current event.
func (e *SerialEngine) CurrentTime() VTimeInSec {
	return e.time
}

// SetCurrentTime sets the current time of the engine.
func (e *SerialEngine) SetCurrentTime(t VTimeInSec) {
	e.time = t
}
