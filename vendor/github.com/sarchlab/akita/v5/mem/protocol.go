package mem

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// AccessReq abstracts read and write requests sent to cache modules or memory
// controllers.
type AccessReq interface {
	sim.Msg
	GetAddress() uint64
	GetByteSize() uint64
	GetPID() vm.PID
}

// AccessRsp abstracts response messages in the memory system.
type AccessRsp interface {
	sim.Msg
}

// ReadReq is a read request sent to a memory controller.
type ReadReq struct {
	sim.MsgMeta
	Address            uint64
	AccessByteSize     uint64
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{} `json:"-"`
}

// GetByteSize returns the number of bytes that the request is accessing.
func (r *ReadReq) GetByteSize() uint64 {
	return r.AccessByteSize
}

// GetAddress returns the address that the request is accessing.
func (r *ReadReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the process ID that the request is working on.
func (r *ReadReq) GetPID() vm.PID {
	return r.PID
}

// WriteReq is a write request sent to a memory controller.
type WriteReq struct {
	sim.MsgMeta
	Address            uint64
	Data               []byte
	DirtyMask          []bool
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{} `json:"-"`
}

// GetByteSize returns the number of bytes that the request is writing.
func (r *WriteReq) GetByteSize() uint64 {
	return uint64(len(r.Data))
}

// GetAddress returns the address that the request is accessing.
func (r *WriteReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the PID of the read address.
func (r *WriteReq) GetPID() vm.PID {
	return r.PID
}

// DataReadyRsp is a response carrying data loaded from memory.
type DataReadyRsp struct {
	sim.MsgMeta
	Data []byte
}

// WriteDoneRsp is a response indicating a write request is completed.
type WriteDoneRsp struct {
	sim.MsgMeta
}

// ControlCommand enumerates control operations for memory components.
type ControlCommand int

const (
	CmdFlush      ControlCommand = iota // Write back dirty data
	CmdInvalidate                       // Invalidate entries without writeback
	CmdDrain                            // Wait for in-flight ops to complete
	CmdReset                            // Soft reset
	CmdPause                            // Disable further processing
	CmdEnable                           // Re-enable processing
)

// ControlReq is a unified control request for all memory components.
type ControlReq struct {
	sim.MsgMeta
	Command         ControlCommand
	DiscardInflight bool     // For Flush: discard vs wait for in-flight
	InvalidateAfter bool     // For Flush: invalidate lines after writeback
	PauseAfter      bool     // For Flush/Drain: pause after completion
	Addresses       []uint64 // For Invalidate: specific addresses (empty = all)
	PID             vm.PID   // For Invalidate: process filter
}

// ControlRsp is the unified response to a ControlReq.
type ControlRsp struct {
	sim.MsgMeta
	Command ControlCommand
	Success bool
}
