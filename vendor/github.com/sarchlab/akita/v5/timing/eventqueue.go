package timing

import (
	"sort"
	"sync"
)

// EventQueue is a queue of events ordered by event time.
type EventQueue interface {
	Push(evt Event)
	Pop() Event
	Len() int
	Peek() Event
}

// queuedEvent pairs an event with the sequence number it was assigned when
// pushed. Events at the same time are ordered by sequence — i.e. in schedule
// order (FIFO) — giving a total, deterministic order that is reproducible after
// a checkpoint is saved and restored.
type queuedEvent struct {
	event Event
	seq   uint64
}

// eventHeap is a typed binary min-heap of queued events ordered by (time, seq).
// It is implemented directly rather than through container/heap so that pushing
// a queuedEvent never boxes it into an interface{} (which would allocate on
// every schedule).
type eventHeap []queuedEvent

func (h eventHeap) less(i, j int) bool {
	a, b := h[i], h[j]
	if a.event.Time() != b.event.Time() {
		return a.event.Time() < b.event.Time()
	}
	return a.seq < b.seq
}

func (h eventHeap) up(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !h.less(i, parent) {
			break
		}
		h[i], h[parent] = h[parent], h[i]
		i = parent
	}
}

func (h eventHeap) down(i, n int) {
	for {
		left := 2*i + 1
		if left >= n {
			break
		}
		smallest := left
		if right := left + 1; right < n && h.less(right, left) {
			smallest = right
		}
		if !h.less(smallest, i) {
			break
		}
		h[i], h[smallest] = h[smallest], h[i]
		i = smallest
	}
}

// EventQueueImpl provides a thread-safe event queue.
type EventQueueImpl struct {
	sync.Mutex

	events  eventHeap
	nextSeq uint64
}

// NewEventQueue creates and returns a newly created EventQueue.
func NewEventQueue() *EventQueueImpl {
	return &EventQueueImpl{events: make(eventHeap, 0)}
}

// Push adds an event to the event queue.
func (q *EventQueueImpl) Push(evt Event) {
	q.Lock()
	q.events = append(q.events, queuedEvent{event: evt, seq: q.nextSeq})
	q.nextSeq++
	q.events.up(len(q.events) - 1)
	q.Unlock()
}

// Pop returns the next earliest event.
func (q *EventQueueImpl) Pop() Event {
	q.Lock()
	evt := popHeap(&q.events)
	q.Unlock()

	return evt
}

// Len returns the number of events in the queue.
func (q *EventQueueImpl) Len() int {
	q.Lock()
	l := len(q.events)
	q.Unlock()

	return l
}

// Peek returns the event in front of the queue without removing it from the
// queue.
func (q *EventQueueImpl) Peek() Event {
	q.Lock()
	evt := q.events[0].event
	q.Unlock()

	return evt
}

// unsafeEventQueue is a lock-free event queue for single-threaded use.
// It implements the EventQueue interface but without mutex overhead.
// Used by SerialEngine where the Run loop is inherently single-threaded.
type unsafeEventQueue struct {
	events  eventHeap
	nextSeq uint64
}

// newUnsafeEventQueue creates an unsafeEventQueue for single-threaded use.
func newUnsafeEventQueue() *unsafeEventQueue {
	return &unsafeEventQueue{events: make(eventHeap, 0)}
}

// Push adds an event to the event queue without locking.
func (q *unsafeEventQueue) Push(evt Event) {
	q.events = append(q.events, queuedEvent{event: evt, seq: q.nextSeq})
	q.nextSeq++
	q.events.up(len(q.events) - 1)
}

// Pop returns the next earliest event without locking.
func (q *unsafeEventQueue) Pop() Event {
	return popHeap(&q.events)
}

// Len returns the number of events in the queue without locking.
func (q *unsafeEventQueue) Len() int {
	return len(q.events)
}

// Peek returns the event in front of the queue without removing it.
func (q *unsafeEventQueue) Peek() Event {
	return q.events[0].event
}

// snapshot returns the queue's events in pop order — by time, then schedule
// order — without modifying the queue. Used to checkpoint the queue.
func (q *unsafeEventQueue) snapshot() []Event {
	sorted := append(eventHeap(nil), q.events...)
	sort.Slice(sorted, func(i, j int) bool { return sorted.less(i, j) })

	out := make([]Event, len(sorted))
	for i := range sorted {
		out[i] = sorted[i].event
	}

	return out
}

// restore pushes events as if freshly scheduled, re-assigning sequence numbers
// in input order. Given events in pop order, this reproduces the original
// (time, seq) ordering. The queue should be empty.
func (q *unsafeEventQueue) restore(events []Event) {
	for _, e := range events {
		q.Push(e)
	}
}

// popHeap removes and returns the earliest event from the heap.
func popHeap(h *eventHeap) Event {
	events := *h
	n := len(events)
	root := events[0].event

	events[0] = events[n-1]
	events[n-1] = queuedEvent{}
	events = events[:n-1]
	*h = events

	if len(events) > 0 {
		events.down(0, len(events))
	}

	return root
}
