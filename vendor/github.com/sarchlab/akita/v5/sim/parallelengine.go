package sim

import (
	"log"
	"math"
	"reflect"
	"runtime"
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
)

// A ParallelEngine is an event engine that is capable for scheduling event
// in a parallel fashion
type ParallelEngine struct {
	hooking.HookableBase

	pauseLock              sync.Mutex
	nowLock                sync.RWMutex
	now                    VTimeInSec
	runningSecondaryEvents bool

	eventChan    chan Event
	waitGroup    sync.WaitGroup
	maxGoRoutine int

	queues             []EventQueue
	queueChan          chan EventQueue
	secondaryQueues    []EventQueue
	secondaryQueueChan chan EventQueue

	registry map[string]Handler
}

// NewParallelEngine creates a ParallelEngine
func NewParallelEngine() *ParallelEngine {
	e := new(ParallelEngine)

	e.eventChan = make(chan Event, 10000)

	e.maxGoRoutine = runtime.GOMAXPROCS(0)
	numQueues := runtime.GOMAXPROCS(0)

	e.registry = make(map[string]Handler)

	// e.spawnWorkers()
	e.queues = make([]EventQueue, 0, numQueues)
	e.queueChan = make(chan EventQueue, numQueues)
	e.secondaryQueues = make([]EventQueue, 0, numQueues)
	e.secondaryQueueChan = make(chan EventQueue, numQueues)

	for i := 0; i < numQueues; i++ {
		queue := NewEventQueue()
		//queue := NewInsertionQueue()
		e.queueChan <- queue

		e.queues = append(e.queues, queue)

		secondaryQueue := NewEventQueue()
		e.secondaryQueueChan <- secondaryQueue

		e.secondaryQueues = append(e.secondaryQueues, secondaryQueue)
	}

	return e
}

// RegisterHandler registers a handler with the given name.
func (e *ParallelEngine) RegisterHandler(name string, handler Handler) {
	e.registry[name] = handler
}

// func (e *ParallelEngine) spawnWorkers() {
// 	for i := 0; i < e.maxGoRoutine; i++ {
// 		go e.worker()
// 	}
// }

// func (e *ParallelEngine) worker() {
// 	for evt := range e.eventChan {
// 		now := e.readNow()

// 		hookCtx := hooking.HookCtx{
// 			Domain: e,
// 			Now:    now,
// 			Pos:    HookPosBeforeEvent,
// 			Item:   evt,
// 		}
// 		e.InvokeHook(&hookCtx)

// 		handler := e.registry[evt.HandlerID()]
// 		_ = handler.Handle(evt)

// 		hookCtx.Pos = HookPosAfterEvent
// 		e.InvokeHook(&hookCtx)

// 		e.waitGroup.Done()
// 	}
// }

func (e *ParallelEngine) readNow() VTimeInSec {
	var now VTimeInSec

	e.nowLock.RLock()
	now = e.now
	e.nowLock.RUnlock()

	return now
}

func (e *ParallelEngine) writeNow(t VTimeInSec) {
	e.nowLock.Lock()
	e.now = t
	e.nowLock.Unlock()
}

// Schedule register an event to be happen in the future
func (e *ParallelEngine) Schedule(evt Event) {
	now := e.readNow()
	if evt.Time() < now {
		log.Panicf(
			"cannot schedule event in the past, evt %s @ %d, now %d",
			reflect.TypeOf(evt), evt.Time(), now)
	}

	if evt.IsSecondary() {
		queue := <-e.secondaryQueueChan
		queue.Push(evt)

		e.secondaryQueueChan <- queue

		return
	}

	queue := <-e.queueChan
	queue.Push(evt)

	e.queueChan <- queue
}

// Run processes all the events scheduled in the SerialEngine
func (e *ParallelEngine) Run() error {
	for {
		if !e.hasMoreEvents() {
			return nil
		}

		e.pauseLock.Lock()
		e.determineWhatToRun()
		e.runRound()

		e.pauseLock.Unlock()
	}
}

func (e *ParallelEngine) determineWhatToRun() {
	primaryTime := e.earliestTimeInQueueGroup(e.queues)
	secondaryTime := e.earliestTimeInQueueGroup(e.secondaryQueues)

	if primaryTime <= secondaryTime {
		e.runningSecondaryEvents = false
		e.writeNow(primaryTime)

		return
	}

	e.runningSecondaryEvents = true
	e.writeNow(secondaryTime)
}

func (e *ParallelEngine) earliestTimeInQueueGroup(
	queues []EventQueue,
) VTimeInSec {
	earliestTime := VTimeInSec(math.MaxUint64)

	for _, q := range queues {
		if q.Len() == 0 {
			continue
		}

		t := q.Peek().Time()
		if t < earliestTime {
			earliestTime = t
		}
	}

	return earliestTime
}

func (e *ParallelEngine) runRound() {
	queues := e.queues
	queueChan := e.queueChan

	if e.runningSecondaryEvents {
		queues = e.secondaryQueues
		queueChan = e.secondaryQueueChan
	}

	e.emptyQueueChan(queues, queueChan)
	e.runEventsUntilConflict(queues, queueChan)
	e.waitGroup.Wait()
}

func (e *ParallelEngine) emptyQueueChan(
	queues []EventQueue,
	queueChan chan EventQueue,
) {
	for range queues {
		<-queueChan
	}
}

func (e *ParallelEngine) hasMoreEvents() bool {
	if e.hasMorePrimaryEvents() || e.hasMoreSecondaryEvents() {
		return true
	}

	return false
}

func (e *ParallelEngine) hasMorePrimaryEvents() bool {
	for _, q := range e.queues {
		if q.Len() > 0 {
			return true
		}
	}

	return false
}

func (e *ParallelEngine) hasMoreSecondaryEvents() bool {
	for _, q := range e.secondaryQueues {
		if q.Len() > 0 {
			return true
		}
	}

	return false
}

func (e *ParallelEngine) runEventsUntilConflict(
	queues []EventQueue,
	queueChan chan EventQueue,
) {
	now := e.readNow()

	for _, queue := range queues {
		for queue.Len() > 0 {
			evt := queue.Peek()
			if evt.Time() == now {
				queue.Pop()
				//e.runEvent(evt)
				e.runEventWithTempWorker(evt)
			} else if evt.Time() < now {
				log.Panicf(
					"cannot run event in the past, evt %s @ %d, now %d",
					reflect.TypeOf(evt), evt.Time(), now)
			} else {
				break
			}
		}

		queueChan <- queue
	}
}

// func (e *ParallelEngine) runEvent(evt Event) {
// 	e.waitGroup.Add(1)

// 	e.eventChan <- evt
// }

func (e *ParallelEngine) runEventWithTempWorker(evt Event) {
	e.waitGroup.Add(1)

	go e.tempWorkerRun(evt)
}

func (e *ParallelEngine) tempWorkerRun(evt Event) {
	now := e.readNow()

	if evt.Time() < now {
		log.Panic("running event in the past")
	}

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

	e.waitGroup.Done()
}

// Pause will prevent the engine to move forward. For events that is scheduled
// at the same time, they may still be triggered.
func (e *ParallelEngine) Pause() {
	e.pauseLock.Lock()
}

// Continue allows the engine to continue to make progress.
func (e *ParallelEngine) Continue() {
	e.pauseLock.Unlock()
}

// CurrentTime returns the current time at which the engine is at.
// Specifically, the run time of the current event.
func (e *ParallelEngine) CurrentTime() VTimeInSec {
	return e.readNow()
}
