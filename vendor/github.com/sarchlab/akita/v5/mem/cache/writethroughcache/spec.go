package writethroughcache

import (
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the writethroughcache.
type Spec struct {
	Freq                  sim.Freq `json:"freq"`
	NumReqPerCycle        int      `json:"num_req_per_cycle"`
	Log2BlockSize         uint64 `json:"log2_block_size"`
	BankLatency           int    `json:"bank_latency"`
	WayAssociativity      int    `json:"way_associativity"`
	MaxNumConcurrentTrans int    `json:"max_num_concurrent_trans"`
	NumBanks              int    `json:"num_banks"`
	NumMSHREntry          int    `json:"num_mshr_entry"`
	NumSets               int    `json:"num_sets"`
	TotalByteSize         uint64 `json:"total_byte_size"`
	DirLatency            int    `json:"dir_latency"`
	MissFillExtraLatency  int    `json:"miss_fill_extra_latency"`

	// WritePolicyType selects the write-policy strategy.
	// Valid values: "write-around" (default), "write-evict", "write-through".
	WritePolicyType string `json:"write_policy_type"`

	// Address mapper configuration (inlined from interface)
	AddressMapperType string   `json:"address_mapper_type"`
	RemotePortNames   []string `json:"remote_port_names"`
	InterleavingSize  uint64   `json:"interleaving_size"`
}
