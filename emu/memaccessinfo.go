package emu

import "gitlab.com/yaotsu/gcn3/disasm"

// MemAccessInfo is the information attached to memory access request.
// When a memory access request returns, the info will be available and the
// ComputeUnit would know what need to do next.
type MemAccessInfo struct {
	IsInstFetch    bool
	WfScheduleInfo *WfScheduleInfo
	Ready          bool

	RegToSet *disasm.Reg
	wiFlatID int
}
