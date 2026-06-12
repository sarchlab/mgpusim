package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// State contains mutable runtime data for the writethroughcache.
type State struct {
	DirectoryState cache.DirectoryState `json:"directory_state"`
	MSHRState      cache.MSHRState      `json:"mshr_state"`

	// Transactions stores all transaction states as a flat list.
	Transactions []transactionState `json:"transactions"`

	DirBuf        queueing.Buffer[int]     `json:"dir_buf"`
	BankBufs      []queueing.Buffer[int]   `json:"bank_bufs"`
	DirPipeline   queueing.Pipeline[int]   `json:"dir_pipeline"`
	DirPostBuf    queueing.Buffer[int]     `json:"dir_post_buf"`
	BankPipelines []queueing.Pipeline[int] `json:"bank_pipelines"`
	BankPostBufs  []queueing.Buffer[int]   `json:"bank_post_bufs"`

	IsPaused bool `json:"is_paused"`

	// Flush request fields (flattened from *mem.ControlReq for serialization)
	HasProcessingFlush bool          `json:"has_processing_flush"`
	ProcessingFlush    flushReqState `json:"processing_flush"`
}

// flushReqState is a serializable representation of a flush control request.
type flushReqState struct {
	MsgMeta         sim.MsgMeta `json:"msg_meta"`
	DiscardInflight bool        `json:"discard_inflight"`
	PauseAfter      bool        `json:"pause_after"`
}

type bankActionType int

const (
	bankActionInvalid bankActionType = iota
	bankActionReadHit
	bankActionWrite
	bankActionWriteFetched
	bankActionReadMissFill // coalesced miss-fill reads
)

// transactionState is the canonical transaction type for the writethroughcache.
// All fields are directly JSON-serializable (no pointers to message types).
type transactionState struct {
	ID uint64 `json:"id"`

	// Read request fields (flattened from *mem.ReadReq)
	HasRead            bool        `json:"has_read"`
	ReadMeta           sim.MsgMeta `json:"read_meta"`
	ReadAddress        uint64      `json:"read_address"`
	ReadAccessByteSize uint64      `json:"read_access_byte_size"`
	ReadPID            vm.PID      `json:"read_pid"`

	// ReadToBottom fields (flattened from *mem.ReadReq)
	HasReadToBottom  bool        `json:"has_read_to_bottom"`
	ReadToBottomMeta sim.MsgMeta `json:"read_to_bottom_meta"`
	ReadToBottomPID  vm.PID      `json:"read_to_bottom_pid"`

	// Write request fields (flattened from *mem.WriteReq)
	HasWrite       bool        `json:"has_write"`
	WriteMeta      sim.MsgMeta `json:"write_meta"`
	WriteAddress   uint64      `json:"write_address"`
	WriteData      []byte      `json:"write_data"`
	WriteDirtyMask []bool      `json:"write_dirty_mask"`
	WritePID       vm.PID      `json:"write_pid"`

	// WriteToBottom fields (flattened from *mem.WriteReq)
	HasWriteToBottom       bool        `json:"has_write_to_bottom"`
	WriteToBottomMeta      sim.MsgMeta `json:"write_to_bottom_meta"`
	WriteToBottomPID       vm.PID      `json:"write_to_bottom_pid"`
	WriteToBottomData      []byte      `json:"write_to_bottom_data"`
	WriteToBottomDirtyMask []bool      `json:"write_to_bottom_dirty_mask"`

	MissFillDelay         int            `json:"miss_fill_delay"`
	BankAction            bankActionType `json:"bank_action"`
	BlockSetID            int            `json:"block_set_id"`
	BlockWayID            int            `json:"block_way_id"`
	HasBlock              bool           `json:"has_block"`
	Data                  []byte         `json:"data"`
	WriteFetchedDirtyMask []bool         `json:"write_fetched_dirty_mask"`

	FetchAndWrite   bool `json:"fetch_and_write"`
	Done            bool `json:"done"`
	BottomWriteDone bool `json:"bottom_write_done"`
	BankDone        bool `json:"bank_done"`

	// Removed marks a transaction that has been completed
	// and removed from active processing.
	Removed bool `json:"removed"`
}

func (t *transactionState) Address() uint64 {
	if t.HasRead {
		return t.ReadAddress
	}

	return t.WriteAddress
}

func (t *transactionState) PID() vm.PID {
	if t.HasRead {
		return t.ReadPID
	}

	return t.WritePID
}
