package timing

import (
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// WfState marks what state that wavefront it in.
type WfState int

// A list of all possible WfState
const (
	WfDispatching WfState = iota // Dispatching in progress, not ready to run
	WfReady                      // Allow the scheduler to schedule instruction
	WfFetching                   // Fetch request sent, but not returned
	WfFetched                    // Instruction fetched, but not issued
	WfRunning                    // Instruction in fight
	WfCompleted                  // Wavefront completed
)

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront
	sync.RWMutex

	WG *WorkGroup

	CodeObject    *insts.HsaCo
	Packet        *kernels.HsaKernelDispatchPacket
	PacketAddress uint64

	State          WfState
	inst           *Inst           // The instruction that is being executed
	scratchpad     emu.Scratchpad  // A temp data buf that is shared by different stages
	LastFetchTime  core.VTimeInSec // The time that the last instruction was fetched
	CompletedLanes int             // The number of lanes that is completed in the SIMD unit

	FetchBuffer []byte
	SIMDID      int
	SRegOffset  int
	VRegOffset  int
	LDSOffset   int

	PC   uint64
	EXEC uint64
	VCC  uint64
	M0   uint32
	SCC  uint8
}

// NewWavefront creates a new Wavefront of the timing package, wrapping the
// wavefront from the kernels package.
func NewWavefront(raw *kernels.Wavefront) *Wavefront {
	wf := new(Wavefront)
	wf.Wavefront = raw

	wf.scratchpad = make([]byte, 4096)

	return wf
}

// Inst return the instruction that is being simulated
func (wf *Wavefront) Inst() *insts.Inst {
	if wf.inst == nil {
		return nil
	}
	return wf.inst.Inst
}

// ManagedInst returns the wrapped Inst
func (wf *Wavefront) ManagedInst() *Inst {
	return wf.inst
}

// Scratchpad returns the scratchpad of the wavefront
func (wf *Wavefront) Scratchpad() emu.Scratchpad {
	return wf.scratchpad
}
