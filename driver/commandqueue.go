package driver

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/mem/vm"
)

// A Command is a task to execute later
type Command interface {
	GetID() string
	GetReqs() []akita.Req
}

// A MemCopyH2DCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyH2DCommand struct {
	ID   string
	Dst  GPUPtr
	Src  interface{}
	Reqs []akita.Req
}

// GetID returns the ID of the command
func (c *MemCopyH2DCommand) GetID() string {
	return c.ID
}

// GetReq returns the request associated with the command
func (c *MemCopyH2DCommand) GetReqs() []akita.Req {
	return c.Reqs
}

// A MemCopyD2HCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyD2HCommand struct {
	ID      string
	Dst     interface{}
	Src     GPUPtr
	RawData []byte
	Reqs    []akita.Req
}

// GetID returns the ID of the command
func (c *MemCopyD2HCommand) GetID() string {
	return c.ID
}

// GetReq returns the request associated with the command
func (c *MemCopyD2HCommand) GetReqs() []akita.Req {
	return c.Reqs
}

// A LaunchKernelCommand is a command will execute a kernel when it is
// processed.
type LaunchKernelCommand struct {
	ID         string
	CodeObject *insts.HsaCo
	GridSize   [3]uint32
	WGSize     [3]uint16
	KernelArgs interface{}
	Packet     *kernels.HsaKernelDispatchPacket
	DPacket    GPUPtr
	Reqs       []akita.Req
}

// GetID returns the ID of the command
func (c *LaunchKernelCommand) GetID() string {
	return c.ID
}

// GetReq returns the request associated with the command
func (c *LaunchKernelCommand) GetReqs() []akita.Req {
	return c.Reqs
}

// A FlushCommand is a command triggers the GPU cache to flush
type FlushCommand struct {
	ID   string
	Reqs []akita.Req
}

// GetID returns the ID of the command
func (c *FlushCommand) GetID() string {
	return c.ID
}

// GetReq returns the request associated with the command
func (c *FlushCommand) GetReqs() []akita.Req {
	return c.Reqs
}

// A NoopCommand is a command that does not do anything. It is used for testing
// purposes.
type NoopCommand struct {
	ID string
}

// GetID returns the ID of the command
func (c *NoopCommand) GetID() string {
	return c.ID
}

// GetReq returns the request associated with the command
func (c *NoopCommand) GetReqs() []akita.Req {
	return nil
}

// A CommandQueue maintains a queue of command where the commands from the
// queue will executes in order.
type CommandQueue struct {
	IsRunning bool
	GPUID     int
	PID       vm.PID
	Context   *Context
	Commands  []Command
	Listeners []*CommandQueueStatusListener
}

// Subscribe returns a CommandQueueStatusListener that listens to the update
// of the command queue
func (q *CommandQueue) Subscribe() *CommandQueueStatusListener {
	l := &CommandQueueStatusListener{
		signal: make(chan bool, 0),
	}

	q.Listeners = append(q.Listeners, l)

	return l
}

// Unsubscribe will unbind a listener to a command queue.
func (q *CommandQueue) Unsubscribe(listener *CommandQueueStatusListener) {
	for i, l := range q.Listeners {
		if l == listener {
			q.Listeners = append(q.Listeners[:i], q.Listeners[i+1:]...)
			return
		}
	}

	panic("not subscribed")
}

// NotifyAllSubscribers will wake up all the subscribers of the command queue
func (q *CommandQueue) NotifyAllSubscribers() {
	fmt.Printf("notifing %d subscribers\n", len(q.Listeners))
	for _, subscriber := range q.Listeners {
		subscriber.Notify()
	}
}

func (d *Driver) Enqueue(q *CommandQueue, c Command) {
	fmt.Printf("enqueueing\n")
	q.Commands = append(q.Commands, c)
	d.enqueueSignal <- true

}

// A CommandQueueStatusListener can be notified when a queue updates its state
type CommandQueueStatusListener struct {
	signal chan bool
}

// Notify triggers the listener who waits for the command queue status update
// continue executing
func (l *CommandQueueStatusListener) Notify() {
	fmt.Printf("listener notified\n")
	l.signal <- true
}

// Wait will block the execution until the command queue updates its status
func (l *CommandQueueStatusListener) Wait() {
	<-l.signal
}
