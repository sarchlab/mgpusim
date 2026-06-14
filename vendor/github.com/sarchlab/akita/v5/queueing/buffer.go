package queueing

import (
	"log"

	"github.com/sarchlab/akita/v5/hooking"
)

// HookPosBufPush marks when an element is pushed into the buffer.
var HookPosBufPush = &hooking.HookPos{Name: "Buffer Push"}

// HookPosBufPop marks when an element is popped from the buffer.
var HookPosBufPop = &hooking.HookPos{Name: "Buffer Pop"}

// Buffer is a generic, bounded FIFO queue. Its state is fully encapsulated:
// callers interact through the methods only, which keeps the capacity and
// FIFO invariants intact and lets the representation evolve freely.
type Buffer[T any] struct {
	hooking.HookableBase `json:"-"`

	name     string
	cap      int
	elements []T
}

// NewBuffer creates a FIFO buffer with the given name and capacity. It returns
// a value so the buffer can be embedded directly in a component's state.
func NewBuffer[T any](name string, capacity int) Buffer[T] {
	return Buffer[T]{
		name: name,
		cap:  capacity,
	}
}

// Name returns the name of the buffer.
func (b *Buffer[T]) Name() string {
	return b.name
}

// Capacity returns the capacity of the buffer.
func (b *Buffer[T]) Capacity() int {
	return b.cap
}

// Size returns the number of elements currently in the buffer.
func (b *Buffer[T]) Size() int {
	return len(b.elements)
}

// CanPush returns true if the buffer has room for another element.
func (b *Buffer[T]) CanPush() bool {
	return len(b.elements) < b.cap
}

// PushTyped adds an element to the back of the buffer. It panics if the buffer
// is already at capacity.
func (b *Buffer[T]) PushTyped(e T) {
	if len(b.elements) >= b.cap {
		log.Panic("buffer overflow")
	}

	b.elements = append(b.elements, e)

	if b.NumHooks() > 0 {
		b.InvokeHook(hooking.HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
		})
	}
}

// Peek returns the element at the front of the buffer without removing it. It
// returns the zero value of T if the buffer is empty.
func (b *Buffer[T]) Peek() T {
	if len(b.elements) == 0 {
		var zero T
		return zero
	}

	return b.elements[0]
}

// UpdateFront replaces the element at the front of the buffer. It is a no-op
// if the buffer is empty. This lets a consumer process the head item in place
// across ticks (for example, marking it committed) without dequeuing it.
func (b *Buffer[T]) UpdateFront(e T) {
	if len(b.elements) == 0 {
		return
	}

	b.elements[0] = e
}

// Pop removes and returns the element at the front of the buffer. It returns
// the zero value of T if the buffer is empty.
func (b *Buffer[T]) Pop() T {
	if len(b.elements) == 0 {
		var zero T
		return zero
	}

	e := b.elements[0]
	b.elements = b.elements[1:]

	if b.NumHooks() > 0 {
		b.InvokeHook(hooking.HookCtx{
			Domain: b,
			Pos:    HookPosBufPop,
			Item:   e,
		})
	}

	return e
}

// Clear removes all elements from the buffer.
func (b *Buffer[T]) Clear() {
	b.elements = nil
}

// Elements returns a copy of the buffer's contents, front to back. It is
// intended for inspection and checkpointing; mutating the returned slice has no
// effect on the buffer.
func (b *Buffer[T]) Elements() []T {
	out := make([]T, len(b.elements))
	copy(out, b.elements)

	return out
}

// Restore replaces the buffer's contents with the given elements (front to
// back) without firing hooks. It is used to rebuild a buffer from a checkpoint
// and panics if the elements exceed the buffer's capacity.
func (b *Buffer[T]) Restore(elements []T) {
	if len(elements) > b.cap {
		log.Panicf("buffer restore overflow: %d elements, capacity %d",
			len(elements), b.cap)
	}

	b.elements = append([]T(nil), elements...)
}
