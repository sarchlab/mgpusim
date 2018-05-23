package timing

import "gitlab.com/yaotsu/gcn3/insts"

// MemAccessAction enumerates all the memory interaction from a compute unit
type MemAccessAction int

// The possible memory access actions
const (
	MemAccessInstFetch MemAccessAction = iota
	MemAccessScalarDataLoad
	MemAccessVectorDataLoad
)

// MemAccessInfo is the information that is attached to a memory access
// request. When the request returns from the memory system, the compute
// unit need the information to perform corresponding action.
type MemAccessInfo struct {
	Action            MemAccessAction
	Wf                *Wavefront
	Dst               *insts.Reg
	Inst              *Inst
	PreCoalescedAddrs [64]uint64
	TotalReqs         int
	ReturnedReqs      int
}
