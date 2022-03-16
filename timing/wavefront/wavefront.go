package wavefront

import (
	"sync"

	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mem/v3/vm"
	"gitlab.com/akita/mgpusim/v2/emu"
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/mgpusim/v2/kernels"
)

// WfState marks what state that wavefront it in.
type WfState int

// A list of all possible WfState
const (
	WfDispatching WfState = iota // Dispatching in progress, not ready to run
	WfReady                      // Allow the scheduler to schedule instruction
	WfRunning                    // Instruction in fight
	WfCompleted                  // Wavefront completed
	WfAtBarrier                  // Wavefront at barrier
)

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront
	sync.RWMutex

	WG *WorkGroup

	pid            vm.PID
	State          WfState
	inst           *Inst          // The instruction that is being executed
	scratchpad     emu.Scratchpad // A temp data buf that is shared by different stages
	LastFetchTime  sim.VTimeInSec // The time that the last instruction was fetched
	CompletedLanes int            // The number of lanes that is completed in the SIMD unit

	InstBuffer        []byte
	InstBufferStartPC uint64
	IsFetching        bool
	InstToIssue       *Inst

	SIMDID     int
	SRegOffset int
	VRegOffset int
	LDSOffset  int

	PC   uint64
	EXEC uint64
	VCC  uint64
	M0   uint32
	SCC  uint8

	OutstandingScalarMemAccess int
	OutstandingVectorMemAccess int
}

// NewWavefront creates a new Wavefront of the timing package, wrapping the
// wavefront from the kernels package.
func NewWavefront(raw *kernels.Wavefront) *Wavefront {
	wf := new(Wavefront)
	wf.Wavefront = raw

	wf.scratchpad = make([]byte, 4096)
	wf.InstBuffer = make([]byte, 0, 256)

	return wf
}

// Inst return the instruction that is being simulated
func (wf *Wavefront) Inst() *insts.Inst {
	if wf.inst == nil {
		return nil
	}
	return wf.inst.Inst
}

// DynamicInst returns the insts with an ID
func (wf *Wavefront) DynamicInst() *Inst {
	return wf.inst
}

// SetDynamicInst sets the dynamic inst to execute
func (wf *Wavefront) SetDynamicInst(i *Inst) {
	wf.inst = i
}

// ManagedInst returns the wrapped Inst
func (wf *Wavefront) ManagedInst() *Inst {
	return wf.inst
}

// Scratchpad returns the scratchpad of the wavefront
func (wf *Wavefront) Scratchpad() emu.Scratchpad {
	return wf.scratchpad
}

// PID returns pid
func (wf *Wavefront) PID() vm.PID {
	return wf.pid
}

// SetPID sets pid
func (wf *Wavefront) SetPID(pid vm.PID) {
	wf.pid = pid
}
