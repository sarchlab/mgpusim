package driver

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

// A Command is a task to execute later
type Command interface {
	GetReq() akita.Req
}

// A MemCopyH2DCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyH2DCommand struct {
	Dst GPUPtr
	Src interface{}
	Req *gcn3.MemCopyH2DReq
}

// GetReq returns the request assocated with the command
func (c *MemCopyH2DCommand) GetReq() akita.Req {
	return c.Req
}

// A MemCopyD2HCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyD2HCommand struct {
	Dst interface{}
	Src GPUPtr
	Req *gcn3.MemCopyD2HReq
}

// GetReq returns the request assocated with the command
func (c *MemCopyD2HCommand) GetReq() akita.Req {
	return c.Req
}

// A LaunchKernelCommand is a command will execute a kernel when it is
// processed.
type LaunchKernelCommand struct {
	CodeObject *insts.HsaCo
	GridSize   [3]uint32
	WGSize     [3]uint16
	KernelArgs interface{}
	Packet     *kernels.HsaKernelDispatchPacket
	DPacket    GPUPtr
	Req        *gcn3.LaunchKernelReq
}

// GetReq returns the request assocated with the command
func (c *LaunchKernelCommand) GetReq() akita.Req {
	return c.Req
}

// A FlushCommand is a command triggers the GPU cache to flush
type FlushCommand struct {
	Req *gcn3.FlushCommand
}

// GetReq returns the request assocated with the command
func (c *FlushCommand) GetReq() akita.Req {
	return c.Req
}

// A CommandQueue maintains a queue of command where the commands from the
// queue will executes in order.
type CommandQueue struct {
	IsRunning bool
	GPUID     int
	Commands  []Command
}

// CreateCommandQueue creates a command queue in the driver
func (d *Driver) CreateCommandQueue() *CommandQueue {
	q := new(CommandQueue)
	q.GPUID = d.usingGPU
	d.CommandQueues = append(d.CommandQueues, q)
	return q
}
