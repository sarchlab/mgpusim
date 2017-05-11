package cu

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
)

// A Builder can construct a fully functional ComputeUnit to the outside world.
// It simplify the compute unit building process.
type Builder struct {
	Engine    core.Engine
	Freq      core.Freq
	CUName    string
	SIMDCount int
	VGPRCount []int
	SGPRCount int
}

// NewBuilder returns a default builder object
func NewBuilder() *Builder {
	b := new(Builder)
	b.Freq = 800 * core.MHz
	b.SIMDCount = 4
	b.SGPRCount = 2048
	b.VGPRCount = []int{16384, 16384, 16384, 16384}
	return b
}

// Build returns a newly constrcted compute unit according to the configuration
func (b *Builder) Build() *ComputeUnit {
	computeUnit := NewComputeUnit(b.CUName)

	computeUnit.Scheduler = b.initScheduler()
	b.initRegFiles(computeUnit)
	b.connect(computeUnit)

	return computeUnit
}

func (b *Builder) initScheduler() *Scheduler {
	wgMapper := NewWGMapper(b.SIMDCount)
	wfDispatcher := new(WfDispatcherImpl)
	scheduler := NewScheduler(b.CUName+".scheduler", b.Engine, wgMapper,
		wfDispatcher, nil, nil)
	scheduler.Freq = b.Freq
	wfDispatcher.Scheduler = scheduler
	return scheduler
}

func (b *Builder) initRegFiles(computeUnit *ComputeUnit) {
	var storage *mem.Storage
	var regFile *RegCtrl
	for i := 0; i < b.SIMDCount; i++ {
		storage = mem.NewStorage(uint64(b.VGPRCount[i] * 4))
		regFile = NewRegCtrl(b.CUName+".vgprs"+string(i), storage, b.Engine)
		computeUnit.VRegFiles = append(computeUnit.VRegFiles, regFile)
	}

	storage = mem.NewStorage(uint64(b.SGPRCount * 4))
	regFile = NewRegCtrl(b.CUName+".sgprs", storage, b.Engine)
	computeUnit.SRegFile = regFile
}

// connect uses a direct connection to connect all the internal component of
// the compute unit.
//
// Since direct connection is the default connection to use, no latency is
// considered. However, users can overwrite this function to use other type of
// connections inside the compute unit
func (b *Builder) connect(computeUnit *ComputeUnit) {
	connection := core.NewDirectConnection()
	core.PlugIn(computeUnit.Scheduler, "ToSReg", connection)
	core.PlugIn(computeUnit.Scheduler, "ToVRegs", connection)

	for i := 0; i < b.SIMDCount; i++ {
		core.PlugIn(computeUnit.VRegFiles[i], "ToOutside", connection)
	}
	core.PlugIn(computeUnit.SRegFile, "ToOutside", connection)
}
