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
	computeUnit := NewComputeUnit(b.CUName, b.Engine)

	return computeUnit
}
