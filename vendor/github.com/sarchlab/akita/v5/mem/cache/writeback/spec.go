package writeback

import (
	"github.com/sarchlab/akita/v5/sim"
)

type cacheState int

const (
	cacheStateInvalid cacheState = iota
	cacheStateRunning
	cacheStatePreFlushing
	cacheStateFlushing
	cacheStatePaused
)

// Spec contains immutable configuration for the writeback cache.
type Spec struct {
	Freq                sim.Freq `json:"freq"`
	NumReqPerCycle      int      `json:"num_req_per_cycle"`
	Log2BlockSize       uint64 `json:"log2_block_size"`
	BankLatency         int    `json:"bank_latency"`
	WayAssociativity    int    `json:"way_associativity"`
	NumBanks            int    `json:"num_banks"`
	NumSets             int    `json:"num_sets"`
	NumMSHREntry        int    `json:"num_mshr_entry"`
	TotalByteSize       uint64 `json:"total_byte_size"`
	DirLatency          int    `json:"dir_latency"`
	WriteBufferCapacity int    `json:"write_buffer_capacity"`
	MaxInflightFetch    int    `json:"max_inflight_fetch"`
	MaxInflightEviction int    `json:"max_inflight_eviction"`

	// Address mapper configuration (inlined from interface)
	AddressMapperType string   `json:"address_mapper_type"`
	RemotePortNames   []string `json:"remote_port_names"`
	InterleavingSize  uint64   `json:"interleaving_size"`
}
