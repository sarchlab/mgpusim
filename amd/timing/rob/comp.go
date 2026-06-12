// Package rob implements a reorder buffer for memory requests. The reorder
// buffer forwards every request that arrives on its Top port to a configured
// bottom unit through its Bottom port, tracks the resulting transactions in
// FIFO order, and releases responses to the Top port only when the
// head-of-line transaction has completed.
package rob

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains the immutable configuration of a reorder buffer.
type Spec struct {
	Freq           timing.Freq `json:"freq"`
	BufferSize     int         `json:"buffer_size"`
	NumReqPerCycle int         `json:"num_req_per_cycle"`

	// BottomUnit is the remote port of the unit that the reorder buffer
	// forwards requests to (e.g., the address translator's Top port). The
	// reorder buffer rewrites the Dst of every forwarded request to this
	// value.
	BottomUnit messaging.RemotePort `json:"bottom_unit"`
}

// transactionState captures everything the reorder buffer needs to remember
// between forwarding a request to the bottom unit and releasing the matching
// response back to the top.
type transactionState struct {
	// ReqFromTopID is the ID of the original request received on the Top
	// port. The response sent back to the top sets RspTo to this value.
	ReqFromTopID uint64 `json:"req_from_top_id"`

	// ReqFromTopSrc is the source port of the original request, used as the
	// destination of the response.
	ReqFromTopSrc messaging.RemotePort `json:"req_from_top_src"`

	// ReqToBottomID is the ID of the duplicated request the reorder buffer
	// sent to the bottom unit. The response from the bottom unit references
	// it as RspTo, which lets the reorder buffer locate the transaction.
	ReqToBottomID uint64 `json:"req_to_bottom_id"`

	// IsRead distinguishes read transactions (which return data) from write
	// transactions (which return only an acknowledgment).
	IsRead bool `json:"is_read"`

	// HasRsp is true once the bottom unit's response has been recorded. The
	// reorder buffer releases the head-of-line transaction only after this
	// flag flips.
	HasRsp bool `json:"has_rsp"`

	// RspData carries the payload of a read response from the bottom unit
	// through to the response forwarded to the top.
	RspData []byte `json:"rsp_data,omitempty"`
}

// State contains the mutable runtime data of a reorder buffer.
type State struct {
	Transactions  []transactionState       `json:"transactions"`
	ControlState  memcontrolprotocol.State `json:"control_state"`
	CurrentCmdID  uint64                   `json:"current_cmd_id"`
	CurrentCmdSrc messaging.RemotePort     `json:"current_cmd_src"`
}

// Comp is a reorder buffer component.
type Comp = modeling.Component[Spec, State, modeling.None]
