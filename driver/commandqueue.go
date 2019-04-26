package driver

import (
	"sync"

	"gitlab.com/akita/mem/vm"
)

// A CommandQueue maintains a queue of command where the commands from the
// queue will executes in order.
type CommandQueue struct {
	IsRunning bool
	GPUID     int
	PID       ca.PID
	Context   *Context

	commandsMutex sync.Mutex
	commands      []Command

	listenerMutex sync.Mutex
	listeners     []*CommandQueueStatusListener
}

// Subscribe returns a CommandQueueStatusListener that listens to the update
// of the command queue
func (q *CommandQueue) Subscribe() *CommandQueueStatusListener {
	l := &CommandQueueStatusListener{
		closeSignal: make(chan bool, 0),
		signal:      make(chan bool, 0),
	}

	q.listenerMutex.Lock()
	q.listeners = append(q.listeners, l)
	q.listenerMutex.Unlock()

	return l
}

// Unsubscribe will unbind a listener to a command queue.
func (q *CommandQueue) Unsubscribe(listener *CommandQueueStatusListener) {
	listener.Close()

	q.listenerMutex.Lock()
	defer q.listenerMutex.Unlock()
	for i, l := range q.listeners {
		if l == listener {
			q.listeners = append(q.listeners[:i], q.listeners[i+1:]...)
			return
		}
	}

	panic("not subscribed")
}

// NotifyAllSubscribers will wake up all the subscribers of the command queue
func (q *CommandQueue) NotifyAllSubscribers() {
	q.listenerMutex.Lock()
	defer q.listenerMutex.Unlock()

	for _, subscriber := range q.listeners {
		subscriber.Notify()
	}
}

// Enqueue adds a command to the command queue
func (q *CommandQueue) Enqueue(c Command) {
	q.commandsMutex.Lock()
	q.commands = append(q.commands, c)
	q.commandsMutex.Unlock()
	q.NotifyAllSubscribers()
}

// Dequeue removes a command from the command queue
func (q *CommandQueue) Dequeue() Command {
	q.commandsMutex.Lock()
	cmd := q.commands[0]
	q.commands = q.commands[1:]
	q.commandsMutex.Unlock()
	q.NotifyAllSubscribers()
	return cmd
}

// Peek returns the first command in the command quee
func (q *CommandQueue) Peek() Command {
	q.commandsMutex.Lock()
	defer q.commandsMutex.Unlock()

	if len(q.commands) == 0 {
		return nil
	}

	return q.commands[0]
}

// NumCommand returns the number of commands currently in the command queue
func (q *CommandQueue) NumCommand() int {
	q.commandsMutex.Lock()
	l := len(q.commands)
	q.commandsMutex.Unlock()
	return l
}

// Enqueue adds a command to a command queue and triggers GPUs to start to
// consume the command.
func (d *Driver) Enqueue(q *CommandQueue, c Command) {
	q.Enqueue(c)
	d.enqueueSignal <- true
}

// A CommandQueueStatusListener can be notified when a queue updates its state
type CommandQueueStatusListener struct {
	closeSignal chan bool
	signal      chan bool
}

// Notify triggers the listener who waits for the command queue status update
// continue executing
func (l *CommandQueueStatusListener) Notify() {
	select {
	case <-l.closeSignal:
	case l.signal <- true:
	}
}

// Wait will block the execution until the command queue updates its status
func (l *CommandQueueStatusListener) Wait() {
	<-l.signal
}

// Close stops the listener from listening
func (l *CommandQueueStatusListener) Close() {
	close(l.closeSignal)
}
