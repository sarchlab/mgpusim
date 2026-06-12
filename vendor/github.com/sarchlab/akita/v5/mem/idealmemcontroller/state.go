package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/sim"
)

// inflightTransaction tracks an in-progress memory request with a countdown.
type inflightTransaction struct {
	CycleLeft      int            `json:"cycle_left"`
	Address        uint64         `json:"address"`
	AccessByteSize uint64         `json:"access_byte_size"`
	ReqID          uint64         `json:"req_id"`
	RecvTaskID     uint64         `json:"recv_task_id"`
	IsRead         bool           `json:"is_read"`
	Data           []byte         `json:"data,omitempty"`
	DirtyMask      []bool         `json:"dirty_mask,omitempty"`
	Src            sim.RemotePort `json:"src"`
}

// State contains mutable runtime data for the ideal memory controller.
type State struct {
	InflightTransactions []inflightTransaction `json:"inflight_transactions"`
	CurrentState         string                `json:"current_state"`
	CurrentCmdID         uint64                `json:"current_cmd_id"`
	CurrentCmdSrc        sim.RemotePort        `json:"current_cmd_src"`
}
