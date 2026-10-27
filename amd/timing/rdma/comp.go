// Package rdma provides the implementation of an RDMA engine.
package rdma

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec contains the immutable configuration of the RDMA engine.
type Spec struct {
	// RemoteRDMAAddressTable selects the owner GPU for an outgoing address.
	RemoteRDMAAddressTable mem.AddressMapper `json:"remote_rdma_address_table"`
	// LocalModules selects local memory for requests received from other GPUs.
	LocalModules        mem.AddressMapper `json:"local_modules"`
	Freq                timing.Freq       `json:"freq"`
	BufferSize          int               `json:"buffer_size"`
	IncomingReqPerCycle int               `json:"incoming_req_per_cycle"`
	IncomingRspPerCycle int               `json:"incoming_rsp_per_cycle"`
	OutgoingReqPerCycle int               `json:"outgoing_req_per_cycle"`
	OutgoingRspPerCycle int               `json:"outgoing_rsp_per_cycle"`
}

// transaction records one memory request that the RDMA engine has forwarded
// and whose response has not yet been delivered back to the requester.
type transaction struct {
	SendTaskID uint64 `json:"send_task_id"`
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
}

// Resources is empty; all routing configuration belongs to Spec.
type Resources struct{}

// Comp is an RDMA engine, a component that helps one GPU access the memory on
// another GPU. It forwards memory requests received on its inside ports to
// remote GPUs, and requests received from remote GPUs to local memory
// modules.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Ports struct {
	RDMARequestInside  messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`
	RDMARequestOutside messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
	RDMADataInside     messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
	RDMADataOutside    messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`
	Ctrl               messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/timing/rdma.responder"`
}
type Middlewares struct {
	Ctrl    *ctrlMiddleware
	Forward *forwardMiddleware
}

func (m *Middlewares) Initialize(c *Comp) {
	if err := c.Spec.LocalModules.Validate(); err != nil {
		panic("rdma: LocalModules: " + err.Error())
	}
	if err := c.Spec.RemoteRDMAAddressTable.Validate(); err != nil {
		panic("rdma: RemoteRDMAAddressTable: " + err.Error())
	}

	m.Ctrl = &ctrlMiddleware{comp: c}
	m.Forward = &forwardMiddleware{comp: c}
}
