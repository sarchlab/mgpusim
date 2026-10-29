// Package rdma provides the implementation of an RDMA engine.
package rdma

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains the immutable configuration of the RDMA engine.
type Spec struct {
	Freq                timing.Freq `json:"freq"`
	BufferSize          int         `json:"buffer_size"`
	IncomingReqPerCycle int         `json:"incoming_req_per_cycle"`
	IncomingRspPerCycle int         `json:"incoming_rsp_per_cycle"`
	OutgoingReqPerCycle int         `json:"outgoing_req_per_cycle"`
	OutgoingRspPerCycle int         `json:"outgoing_rsp_per_cycle"`

	// The access counter counts the remote accesses of the GPU (the
	// requests from the local L1 caches) for page migration. With
	// CountAccesses, the engine counts them for the statistics; DeviceMemSize
	// attributes each remote read to the device that serves it. When
	// CounterThreshold is also positive, it counts the accesses to each page
	// of 2^CounterLog2PageSize bytes, and each time the count of a page
	// reaches the threshold it resets the count and sends an
	// AccessCounterNotification from its "AccessCounter" port (declared only
	// then) to CounterNotifyDst.
	CountAccesses       bool                 `json:"count_accesses"`
	DeviceID            uint64               `json:"device_id"`
	DeviceMemSize       uint64               `json:"device_mem_size"`
	CounterLog2PageSize uint64               `json:"counter_log2_page_size"`
	CounterThreshold    uint32               `json:"counter_threshold"`
	CounterNotifyDst    messaging.RemotePort `json:"counter_notify_dst"`
}

// NotifiesAccesses reports whether the access counter sends notifications.
func (s Spec) NotifiesAccesses() bool {
	return s.CountAccesses && s.CounterThreshold > 0
}

// AccessCounterState is the state of the access counter.
type AccessCounterState struct {
	// Counts are the remote accesses to each page (by physical page number)
	// since its last notification.
	Counts map[uint64]uint32 `json:"counts"`

	// Notifications are waiting to be sent.
	Notifications []AccessCounterNotification `json:"notifications"`

	// Statistics: remote accesses, remote reads, and remote reads by the
	// device that serves them.
	RemoteAccesses     uint64            `json:"remote_accesses"`
	RemoteReads        uint64            `json:"remote_reads"`
	RemoteReadsByOwner map[uint64]uint64 `json:"remote_reads_by_owner"`
	NumNotifications   uint64            `json:"num_notifications"`
}

// transaction records one memory request that the RDMA engine has forwarded
// and whose response has not yet been delivered back to the requester.
type transaction struct {
	// OriginalReqID is the ID of the request received by the RDMA engine.
	OriginalReqID uint64 `json:"original_req_id"`

	// OriginalSrc is where the original request came from. The response is
	// sent back to this port.
	OriginalSrc messaging.RemotePort `json:"original_src"`

	// ForwardedReqID is the ID of the cloned request sent onward. Responses
	// are matched against this ID through their RspTo field.
	ForwardedReqID uint64 `json:"forwarded_req_id"`

	// RecvTaskID is the tracing task ID at the receiver side for the
	// original request. Zero when tracing is disabled.
	RecvTaskID uint64 `json:"recv_task_id"`

	// RemoteMilestoneEmitted records whether the "remote" data milestone has
	// been emitted (when the response first arrived), so that egress-port
	// backpressure retries do not emit it again.
	RemoteMilestoneEmitted bool `json:"remote_milestone_emitted"`
}

// State contains the mutable runtime data of the RDMA engine.
type State struct {
	IsDraining              bool                 `json:"is_draining"`
	PauseIncomingReqsFromL1 bool                 `json:"pause_incoming_reqs_from_l1"`
	CurrentDrainReqID       uint64               `json:"current_drain_req_id"`
	CurrentDrainReqSrc      messaging.RemotePort `json:"current_drain_req_src"`

	TransactionsFromInside  []transaction `json:"transactions_from_inside"`
	TransactionsFromOutside []transaction `json:"transactions_from_outside"`

	AccessCounter AccessCounterState `json:"access_counter"`
}

// Resources holds the shared references used by the RDMA engine to route
// memory requests.
type Resources struct {
	// LocalModules finds the local module (e.g., an L2 cache or a memory
	// controller) that serves requests arriving from other GPUs.
	LocalModules mem.AddressToPortMapper

	// RemoteRDMAAddressTable finds the RDMA engine of the GPU that owns a
	// given address.
	RemoteRDMAAddressTable mem.AddressToPortMapper
}

// Comp is an RDMA engine, a component that helps one GPU access the memory on
// another GPU. It forwards memory requests received on its inside ports to
// remote GPUs, and requests received from remote GPUs to local memory
// modules.
type Comp = modeling.Component[Spec, State, Resources]
