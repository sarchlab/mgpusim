package wavefront

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// ExecutionState is a wavefront's register/control state. Parent work-group,
// instruction, scoreboard, and register-file references are bound by the CU.
type ExecutionState struct {
	PID                        vm.PID
	State                      WfState
	LastFetchTime              timing.VTimeInPicoSec
	CompletedLanes             int
	InstBuffer                 []byte
	InstBufferStartPC          uint64
	IsFetching                 bool
	SIMDID                     int
	SRegOffset                 int
	VRegOffset                 int
	LDSOffset                  int
	PC                         uint64
	EXEC                       uint64
	VCC                        uint64
	M0                         uint32
	SCC                        byte
	OutstandingScalarMemAccess int
	OutstandingVectorMemAccess int
	InFlightInsts              int
}

func (wf *Wavefront) SnapshotExecution() ExecutionState {
	return ExecutionState{
		PID:                        wf.pid,
		State:                      wf.State,
		LastFetchTime:              wf.LastFetchTime,
		CompletedLanes:             wf.CompletedLanes,
		InstBuffer:                 wf.InstBuffer,
		InstBufferStartPC:          wf.InstBufferStartPC,
		IsFetching:                 wf.IsFetching,
		SIMDID:                     wf.SIMDID,
		SRegOffset:                 wf.SRegOffset,
		VRegOffset:                 wf.VRegOffset,
		LDSOffset:                  wf.LDSOffset,
		PC:                         wf.pc,
		EXEC:                       wf.exec,
		VCC:                        wf.vcc,
		M0:                         wf.M0,
		SCC:                        wf.scc,
		OutstandingScalarMemAccess: wf.OutstandingScalarMemAccess,
		OutstandingVectorMemAccess: wf.OutstandingVectorMemAccess,
		InFlightInsts:              wf.InFlightInsts,
	}
}
func (wf *Wavefront) RestoreExecution(s ExecutionState) {
	wf.pid = s.PID
	wf.State = s.State
	wf.LastFetchTime = s.LastFetchTime
	wf.CompletedLanes = s.CompletedLanes
	wf.InstBuffer = s.InstBuffer
	wf.InstBufferStartPC = s.InstBufferStartPC
	wf.IsFetching = s.IsFetching
	wf.SIMDID = s.SIMDID
	wf.SRegOffset = s.SRegOffset
	wf.VRegOffset = s.VRegOffset
	wf.LDSOffset = s.LDSOffset
	wf.pc = s.PC
	wf.exec = s.EXEC
	wf.vcc = s.VCC
	wf.M0 = s.M0
	wf.scc = s.SCC
	wf.OutstandingScalarMemAccess = s.OutstandingScalarMemAccess
	wf.OutstandingVectorMemAccess = s.OutstandingVectorMemAccess
	wf.InFlightInsts = s.InFlightInsts
}
