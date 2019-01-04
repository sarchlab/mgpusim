package driver

import (
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

// A CommandQueue maintains a queue of command where the commands from the
// queue will executes in order.
type CommandQueue struct {
	IsRunning bool
	GPUID     int
	PID       vm.PID
	Commands  []Command
}

// CreateCommandQueue creates a command queue in the driver
func (d *Driver) CreateCommandQueue() *CommandQueue {
	q := new(CommandQueue)
	q.GPUID = d.usingGPU
	q.PID = d.currentPID
	d.CommandQueues = append(d.CommandQueues, q)
	return q
}
