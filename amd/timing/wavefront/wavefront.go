package wavefront

import "github.com/sarchlab/mgpusim/v5/amd/insts"

// RegFileAccessor provides access to register files for a wavefront.
type RegFileAccessor interface {
	ReadReg(reg *insts.Reg, regCount int, laneID int, waveOffset int) []byte
	WriteReg(reg *insts.Reg, regCount int, laneID int, waveOffset int, data []byte)
}

// WfState marks what state that wavefront it in.
type WfState int

// A list of all possible WfState
const (
	WfDispatching    WfState = iota // Dispatching in progress, not ready to run
	WfReady                         // Allow the scheduler to schedule instruction
	WfRunning                       // Instruction in flight
	WfDraining                      // Execution ended; waiting for outstanding prefetch
	WfCompleted                     // Drained; waiting for work-group completion send
	WfAtBarrier                     // Wavefront at barrier
	WfSampledWaiting                // Waiting for the sampled completion event
)
