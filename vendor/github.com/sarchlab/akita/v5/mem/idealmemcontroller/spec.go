package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the ideal memory controller.
type Spec struct {
	Freq          sim.Freq `json:"freq"`
	Width         int      `json:"width"`
	Latency       int      `json:"latency"`
	CacheLineSize int    `json:"cache_line_size"`
	StorageRef    string `json:"storage_ref"`
	AddrConvKind  string `json:"addr_conv_kind"`

	AddrInterleavingSize    uint64 `json:"addr_interleaving_size"`
	AddrTotalNumOfElements  int    `json:"addr_total_num_of_elements"`
	AddrCurrentElementIndex int    `json:"addr_current_element_index"`
	AddrOffset              uint64 `json:"addr_offset"`
}
