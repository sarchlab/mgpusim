package timing

import "gitlab.com/akita/gcn3/insts"

// MemAccessAction enumerates all the memory interaction from a compute unit
type MemAccessAction int

// The possible memory access actions
const (
	MemAccessInstFetch MemAccessAction = iota
	MemAccessScalarDataLoad
	MemAccessVectorDataLoad
	MemAccessVectorDataStore
)

// MemAccessInfo is the information that is attached to a memory access
// request. When the request returns from the memory system, the compute
// unit need the information to perform corresponding action.
type MemAccessInfo struct {
	*InstLevelInfo
	Action            MemAccessAction
	Wf                *Wavefront
	Dst               *insts.Reg
	RegCount          int
	Address           uint64
	PreCoalescedAddrs [64]uint64
}

// InstLevelInfo preserves the information that is shared by multiple requests
// generated from the same instruction
type InstLevelInfo struct {
	Inst         *Inst
	TotalReqs    int
	ReturnedReqs int
}

func newMemAccessInfo() *MemAccessInfo {
	info := new(MemAccessInfo)
	info.InstLevelInfo = new(InstLevelInfo)
	return info
}
