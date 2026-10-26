package driver

import "github.com/sarchlab/akita/v5/mem/vm"

// Context identifies a Runtime-owned address space and device selection.
type Context uint64

// CommandQueue identifies a Runtime-owned ordered stream.
type CommandQueue uint64

// CommandID identifies a submitted command.
type CommandID uint64

// Event identifies a zero-duration timestamp marker.
type Event uint64

type buffer struct {
	Address Ptr
	Size    uint64
	Dirty   bool
}
type contextData struct {
	PID     vm.PID
	GPU     int
	Buffers []buffer
}
type queueData struct {
	Context    Context
	GPU        int
	Pending    []Command
	Unfinished uint64
}
