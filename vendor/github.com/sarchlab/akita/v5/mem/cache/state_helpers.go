package cache

import (
	"github.com/sarchlab/akita/v5/sim"
)

// BlockState is a serializable representation of a cache Block.
type BlockState struct {
	PID          uint32 `json:"pid"`
	Tag          uint64 `json:"tag"`
	WayID        int    `json:"way_id"`
	SetID        int    `json:"set_id"`
	CacheAddress uint64 `json:"cache_address"`
	IsValid      bool   `json:"is_valid"`
	IsDirty      bool   `json:"is_dirty"`
	ReadCount    int    `json:"read_count"`
	IsLocked     bool   `json:"is_locked"`
	DirtyMask    []bool `json:"dirty_mask"`
}

// SetState is a serializable representation of a cache Set.
type SetState struct {
	Blocks   []BlockState `json:"blocks"`
	LRUOrder []int        `json:"lru_order"`
}

// DirectoryState is a serializable representation of a DirectoryImpl.
type DirectoryState struct {
	Sets []SetState `json:"sets"`
}

// MSHREntryState is a serializable representation of an MSHREntry.
type MSHREntryState struct {
	PID                uint32 `json:"pid"`
	Address            uint64 `json:"address"`
	TransactionIndices []int  `json:"transaction_indices"`
	BlockSetID         int    `json:"block_set_id"`
	BlockWayID         int    `json:"block_way_id"`
	HasBlock           bool   `json:"has_block"`
	HasReadReq         bool         `json:"has_read_req"`
	ReadReq            sim.MsgMeta  `json:"read_req"`
	HasDataReady       bool         `json:"has_data_ready"`
	DataReady          sim.MsgMeta  `json:"data_ready"`
	Data               []byte `json:"data"`
}

// GetPID returns the PID of the MSHR entry.
func (e MSHREntryState) GetPID() uint32 { return e.PID }

// GetAddress returns the address of the MSHR entry.
func (e MSHREntryState) GetAddress() uint64 { return e.Address }

// MSHRState is a serializable representation of an mshrImpl.
type MSHRState struct {
	Entries []MSHREntryState `json:"entries"`
}
