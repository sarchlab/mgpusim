package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3/emu"
)

// A Builder can construct a fully functional ComputeUnit to the outside world.
// It simplify the compute unit building process.
type Builder struct {
	Engine    core.Engine
	Freq      util.Freq
	CUName    string
	SIMDCount int
	VGPRCount []int
	SGPRCount int
	Decoder   emu.Decoder
	InstMem   core.Component
	ToInstMem core.Connection
}

// NewBuilder returns a default builder object
func NewBuilder() *Builder {
	b := new(Builder)
	b.Freq = 800 * util.MHz
	b.SIMDCount = 4
	b.SGPRCount = 3200
	b.VGPRCount = []int{16384, 16384, 16384, 16384}
	return b
}

// Build returns a newly constructed compute unit according to the configuration
func (b *Builder) Build() *ComputeUnit {
	cu := NewComputeUnit(b.CUName, b.Engine)
	cu.Freq = b.Freq
	cu.WGMapper = NewWGMapper(cu, 4)
	cu.WfDispatcher = NewWfDispatcher(cu)

	for i := 0; i < 4; i++ {
		cu.WfPools = append(cu.WfPools, NewWavefrontPool(10))
	}

	b.equipScheduler(cu)
	b.connectToInstMem(cu)
	cu.Decoder = b.Decoder

	return cu
}

func (b *Builder) equipScheduler(cu *ComputeUnit) {
	fetchArbitor := new(FetchArbiter)
	issueArbitor := new(IssueArbiter)
	scheduler := NewScheduler(cu, fetchArbitor, issueArbitor)
	cu.Scheduler = scheduler
}

func (b *Builder) connectToInstMem(cu *ComputeUnit) {
	cu.InstMem = b.InstMem
	core.PlugIn(cu, "ToInstMem", b.ToInstMem)
}
