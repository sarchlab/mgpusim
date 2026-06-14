package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec contains immutable configuration for the writethroughcache.
type Spec struct {
	Freq                  timing.Freq `json:"freq"`
	NumReqPerCycle        int         `json:"num_req_per_cycle"`
	Log2BlockSize         uint64      `json:"log2_block_size"`
	BankLatency           int         `json:"bank_latency"`
	WayAssociativity      int         `json:"way_associativity"`
	MaxNumConcurrentTrans int         `json:"max_num_concurrent_trans"`
	NumBanks              int         `json:"num_banks"`
	NumMSHREntry          int         `json:"num_mshr_entry"`
	NumSets               int         `json:"num_sets"`
	TotalByteSize         uint64      `json:"total_byte_size"`
	DirLatency            int         `json:"dir_latency"`

	// WritePolicyType selects the write-policy strategy.
	// Valid values: "write-around" (default), "write-evict", "write-through".
	WritePolicyType string `json:"write_policy_type"`

	// Address mapper configuration (inlined from interface)
	AddressMapperType string   `json:"address_mapper_type"`
	RemotePortNames   []string `json:"remote_port_names"`
	InterleavingSize  uint64   `json:"interleaving_size"`
}

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

	IsPaused      bool                 `json:"is_paused"`
	IsDraining    bool                 `json:"is_draining"`
	CurrentCmdID  uint64               `json:"current_cmd_id"`
	CurrentCmdSrc messaging.RemotePort `json:"current_cmd_src"`
}

type bankActionType int

const (
	bankActionInvalid bankActionType = iota
	bankActionReadHit
	bankActionWrite
	bankActionWriteFetched
)

// transactionState is the canonical transaction type for the writethroughcache.
// All fields are directly JSON-serializable (no pointers to message types).
type transactionState struct {
	ID uint64 `json:"id"`

	// Read request fields (flattened from memprotocol.ReadReq)
	HasRead            bool              `json:"has_read"`
	ReadMeta           messaging.MsgMeta `json:"read_meta"`
	ReadAddress        uint64            `json:"read_address"`
	ReadAccessByteSize uint64            `json:"read_access_byte_size"`
	ReadPID            vm.PID            `json:"read_pid"`

	// ReadToBottom fields (flattened from memprotocol.ReadReq)
	HasReadToBottom  bool              `json:"has_read_to_bottom"`
	ReadToBottomMeta messaging.MsgMeta `json:"read_to_bottom_meta"`
	ReadToBottomPID  vm.PID            `json:"read_to_bottom_pid"`

	// Write request fields (flattened from memprotocol.WriteReq)
	HasWrite       bool              `json:"has_write"`
	WriteMeta      messaging.MsgMeta `json:"write_meta"`
	WriteAddress   uint64            `json:"write_address"`
	WriteData      []byte            `json:"write_data"`
	WriteDirtyMask []bool            `json:"write_dirty_mask"`
	WritePID       vm.PID            `json:"write_pid"`

	// WriteToBottom fields (flattened from memprotocol.WriteReq)
	HasWriteToBottom       bool              `json:"has_write_to_bottom"`
	WriteToBottomMeta      messaging.MsgMeta `json:"write_to_bottom_meta"`
	WriteToBottomPID       vm.PID            `json:"write_to_bottom_pid"`
	WriteToBottomData      []byte            `json:"write_to_bottom_data"`
	WriteToBottomDirtyMask []bool            `json:"write_to_bottom_dirty_mask"`

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

	// WaitForMSHRFill / MSHRFillDone / MSHRFillFetcherIdx track an
	// MSHR-coalesced write whose data is merged into another transaction's
	// fetched line. The coalesced write never visits the bank itself, so
	// completion must wait until the fetcher's bankActionWriteFetched stage
	// has written the merged line into storage.
	WaitForMSHRFill    bool `json:"wait_for_mshr_fill"`
	MSHRFillDone       bool `json:"mshr_fill_done"`
	MSHRFillFetcherIdx int  `json:"mshr_fill_fetcher_idx"`

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

// Resources holds the shared resources and external wiring referenced by the
// writethroughcache. Storage is the (optionally shared) backing storage.
// AddressMapper and RemotePorts describe how the cache reaches the lower-level
// modules; they are only consumed at Build time to populate the Spec's address
// mapper configuration and are not serialized with the component state.
type Resources struct {
	Storage *mem.Storage

	AddressMapper mem.AddressToPortMapper `json:"-"`
	RemotePorts   []messaging.RemotePort  `json:"-"`
}

// Comp is the writethroughcache component type.
type Comp = modeling.Component[Spec, State, Resources]
