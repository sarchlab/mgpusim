package driver

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// A Command is a task to execute later
type Command interface {
	GetID() uint64
	GetReqs() []messaging.Msg
	AddReq(req messaging.Msg)
	RemoveReq(req messaging.Msg)
}

// A MemCopyH2DCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyH2DCommand struct {
	ID   uint64
	Dst  Ptr
	Src  interface{}
	Reqs []messaging.Msg
}

// GetID returns the ID of the command
func (c *MemCopyH2DCommand) GetID() uint64 {
	return c.ID
}

// GetReqs returns the requests associated with the command
func (c *MemCopyH2DCommand) GetReqs() []messaging.Msg {
	return c.Reqs
}

// AddReq adds a request to the request list associated with the command
func (c *MemCopyH2DCommand) AddReq(req messaging.Msg) {
	c.Reqs = append(c.Reqs, req)
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *MemCopyH2DCommand) RemoveReq(req messaging.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
}

// A MemCopyD2HCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyD2HCommand struct {
	ID      uint64
	Dst     interface{}
	Src     Ptr
	RawData []byte
	Reqs    []messaging.Msg
}

// GetID returns the ID of the command
func (c *MemCopyD2HCommand) GetID() uint64 {
	return c.ID
}

// GetReqs returns the request associated with the command
func (c *MemCopyD2HCommand) GetReqs() []messaging.Msg {
	return c.Reqs
}

// AddReq adds a request to the request list associated with the command
func (c *MemCopyD2HCommand) AddReq(req messaging.Msg) {
	c.Reqs = append(c.Reqs, req)
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *MemCopyD2HCommand) RemoveReq(req messaging.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
}

// A LaunchKernelCommand is a command will execute a kernel when it is
// processed.
type LaunchKernelCommand struct {
	ID         uint64
	CodeObject *insts.KernelCodeObject
	GridSize   [3]uint32
	WGSize     [3]uint16
	KernelArgs interface{}
	Packet     *kernels.HsaKernelDispatchPacket
	DPacket    Ptr
	Reqs       []messaging.Msg
}

// GetID returns the ID of the command
func (c *LaunchKernelCommand) GetID() uint64 {
	return c.ID
}

// GetReqs returns the request associated with the command
func (c *LaunchKernelCommand) GetReqs() []messaging.Msg {
	return c.Reqs
}

// AddReq adds a request to the request list associated with the command
func (c *LaunchKernelCommand) AddReq(req messaging.Msg) {
	c.Reqs = append(c.Reqs, req)
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *LaunchKernelCommand) RemoveReq(req messaging.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
}

// A FlushCommand is a command triggers the GPU cache to flush
type FlushCommand struct {
	ID   uint64
	Reqs []messaging.Msg
}

// GetID returns the ID of the command
func (c *FlushCommand) GetID() uint64 {
	return c.ID
}

// GetReqs returns the request associated with the command
func (c *FlushCommand) GetReqs() []messaging.Msg {
	return c.Reqs
}

// AddReq adds a request to the request list associated with the command
func (c *FlushCommand) AddReq(req messaging.Msg) {
	c.Reqs = append(c.Reqs, req)
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *FlushCommand) RemoveReq(req messaging.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
}

// A NoopCommand is a command that does not do anything. It is used for testing
// purposes.
type NoopCommand struct {
	ID uint64
}

// GetID returns the ID of the command
func (c *NoopCommand) GetID() uint64 {
	return c.ID
}

// GetReqs returns the request associated with the command
func (c *NoopCommand) GetReqs() []messaging.Msg {
	return nil
}

// AddReq adds a request to the request list associated with the command
func (c *NoopCommand) AddReq(req messaging.Msg) {
	// No action
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *NoopCommand) RemoveReq(req messaging.Msg) {
	// no action
}

// removeMsgFromMsgList removes the message with the same ID as the given
// message from the list. Messages are value types in Akita v5 and may carry
// non-comparable fields, so they are matched by ID rather than by equality.
func removeMsgFromMsgList(
	msg messaging.Msg,
	msgs []messaging.Msg,
) []messaging.Msg {
	for i, m := range msgs {
		if m.Meta().ID == msg.Meta().ID {
			return append(msgs[:i], msgs[i+1:]...)
		}
	}

	panic("not found")
}

// A LaunchUnifiedMultiGPUKernelCommand is a command that launches a kernel
// on multiple unified GPUs.
type LaunchUnifiedMultiGPUKernelCommand struct {
	ID           uint64
	CodeObject   *insts.KernelCodeObject
	GridSize     [3]uint32
	WGSize       [3]uint16
	KernelArgs   interface{}
	PacketArray  []*kernels.HsaKernelDispatchPacket
	DPacketArray []Ptr
	Reqs         []messaging.Msg
}

// GetID returns the ID of the command
func (c *LaunchUnifiedMultiGPUKernelCommand) GetID() uint64 {
	return c.ID
}

// GetReqs returns the request associated with the command
func (c *LaunchUnifiedMultiGPUKernelCommand) GetReqs() []messaging.Msg {
	return c.Reqs
}

// AddReq adds a request to the request list associated with the command
func (c *LaunchUnifiedMultiGPUKernelCommand) AddReq(req messaging.Msg) {
	c.Reqs = append(c.Reqs, req)
}

// RemoveReq removes a request from the request list associated with the
// command.
func (c *LaunchUnifiedMultiGPUKernelCommand) RemoveReq(req messaging.Msg) {
	c.Reqs = removeMsgFromMsgList(req, c.Reqs)
}
