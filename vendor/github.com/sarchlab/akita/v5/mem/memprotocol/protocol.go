// Package memprotocol defines the memory access protocol: the message types
// exchanged between memory requesters (cores, caches issuing fills) and
// responders (caches, memory controllers), and the protocol roles ports bind
// to.
package memprotocol

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
)

// Protocol is the memory access protocol: requesters issue reads and writes,
// responders (caches, memory controllers) answer with data-ready and
// write-done responses. Defining the protocol registers every message type it
// carries with the checkpoint codec. The Info field on these messages is
// tagged json:"-" and is not checkpointed.
var (
	Protocol = messaging.DefineProtocol("mem",
		messaging.RoleDef{Name: "requester",
			Sends: []messaging.Msg{ReadReq{}, WriteReq{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []messaging.Msg{DataReadyRsp{}, WriteDoneRsp{}}},
	)
	Requester = Protocol.Role("requester")
	Responder = Protocol.Role("responder")
)

// AccessReq abstracts read and write requests sent to cache modules or memory
// controllers.
type AccessReq interface {
	messaging.Msg
	GetAddress() uint64
	GetByteSize() uint64
	GetPID() vm.PID
}

// AccessRsp abstracts response messages in the memory system.
type AccessRsp interface {
	messaging.Msg
}

// ReadReq is a read request sent to a memory controller.
type ReadReq struct {
	messaging.MsgMeta
	Address            uint64
	AccessByteSize     uint64
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{} `json:"-"`
}

// GetByteSize returns the number of bytes that the request is accessing.
func (r ReadReq) GetByteSize() uint64 {
	return r.AccessByteSize
}

// GetAddress returns the address that the request is accessing.
func (r ReadReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the process ID that the request is working on.
func (r ReadReq) GetPID() vm.PID {
	return r.PID
}

// WriteReq is a write request sent to a memory controller.
type WriteReq struct {
	messaging.MsgMeta
	Address            uint64
	Data               []byte
	DirtyMask          []bool
	PID                vm.PID
	CanWaitForCoalesce bool
	Info               interface{} `json:"-"`
}

// GetByteSize returns the number of bytes that the request is writing.
func (r WriteReq) GetByteSize() uint64 {
	return uint64(len(r.Data))
}

// GetAddress returns the address that the request is accessing.
func (r WriteReq) GetAddress() uint64 {
	return r.Address
}

// GetPID returns the PID of the read address.
func (r WriteReq) GetPID() vm.PID {
	return r.PID
}

// DataReadyRsp is a response carrying data loaded from memory.
type DataReadyRsp struct {
	messaging.MsgMeta
	Data []byte
}

// WriteDoneRsp is a response indicating a write request is completed.
type WriteDoneRsp struct {
	messaging.MsgMeta
}
