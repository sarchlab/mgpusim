package tlb

import (
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the TLB.
type Spec struct {
	Freq                       sim.Freq         `json:"freq"`
	NumSets                    int              `json:"num_sets"`
	NumWays                    int              `json:"num_ways"`
	PageSize                   uint64           `json:"page_size"`
	NumReqPerCycle             int              `json:"num_req_per_cycle"`
	MSHRSize                   int              `json:"mshr_size"`
	Latency                    int              `json:"latency"`
	PipelineWidth              int              `json:"pipeline_width"`
	AddrMapperKind             string           `json:"addr_mapper_kind"`
	AddrMapperPorts            []sim.RemotePort `json:"addr_mapper_ports"`
	AddrMapperInterleavingSize uint64           `json:"addr_mapper_interleaving_size"`
}
