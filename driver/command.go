package driver

import (
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/mgpusim/v2/kernels"
)

// A Command is a task to execute later
type Command interface {
	GetID() string
	GetReqs() []sim.Msg
	RemoveReq(req sim.Msg)
}

// A MemCopyH2DCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyH2DCommand struct {
	ID   string
	Dst  GPUPtr
	Src  interface{}
	Reqs []sim.Msg
}

// GetID returns the ID of the command
func (c *MemCopyH2DCommand) GetID() string {
	return c.ID
}

// GetReqs returns the requests associated with the command
func (c *MemCopyH2DCommand) GetReqs() []sim.Msg {
	return c.Reqs
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *MemCopyH2DCommand) RemoveReq(req sim.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
}

// A MemCopyD2HCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyD2HCommand struct {
	ID      string
	Dst     interface{}
	Src     GPUPtr
	RawData []byte
	Reqs    []sim.Msg
}

// GetID returns the ID of the command
func (c *MemCopyD2HCommand) GetID() string {
	return c.ID
}

// GetReqs returns the request associated with the command
func (c *MemCopyD2HCommand) GetReqs() []sim.Msg {
	return c.Reqs
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *MemCopyD2HCommand) RemoveReq(req sim.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
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
	Reqs       []sim.Msg
}

// GetID returns the ID of the command
func (c *LaunchKernelCommand) GetID() string {
	return c.ID
}

// GetReqs returns the request associated with the command
func (c *LaunchKernelCommand) GetReqs() []sim.Msg {
	return c.Reqs
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *LaunchKernelCommand) RemoveReq(req sim.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
}

// A FlushCommand is a command triggers the GPU cache to flush
type FlushCommand struct {
	ID   string
	Reqs []sim.Msg
}

// GetID returns the ID of the command
func (c *FlushCommand) GetID() string {
	return c.ID
}

// GetReqs returns the request associated with the command
func (c *FlushCommand) GetReqs() []sim.Msg {
	return c.Reqs
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *FlushCommand) RemoveReq(req sim.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
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

// GetReqs returns the request associated with the command
func (c *NoopCommand) GetReqs() []sim.Msg {
	return nil
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *NoopCommand) RemoveReq(req sim.Msg) {
	// no action
}

func removeMsgFromMsgList(msg sim.Msg, msgs []sim.Msg) []sim.Msg {
	newMsgs := make([]sim.Msg, 0, len(msgs)-1)
	for _, m := range msgs {
		if m != msg {
			newMsgs = append(newMsgs, m)
		}
	}
	return newMsgs
}
