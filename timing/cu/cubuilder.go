package cu

import (
	"fmt"

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
	Decoder   Decoder
	InstMem   core.Component
	ToInstMem core.Connection
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
	b.initDecodeUnits(computeUnit)
	b.initExecUnits(computeUnit)
	b.initRegFiles(computeUnit)
	b.setUpDependency(computeUnit)
	b.connect(computeUnit)

	return computeUnit
}

func (b *Builder) initScheduler() *Scheduler {
	wgMapper := NewWGMapper(b.SIMDCount)
	wfDispatcher := new(WfDispatcherImpl)
	fetchArbiter := new(FetchArbiter)
	issueArbiter := NewIssueArbiter()
	scheduler := NewScheduler(b.CUName+".scheduler", b.Engine, wgMapper,
		wfDispatcher, fetchArbiter, issueArbiter, b.Decoder)

	scheduler.Freq = b.Freq
	scheduler.InstMem = b.InstMem

	wfDispatcher.Scheduler = scheduler
	return scheduler
}

func (b *Builder) initDecodeUnits(computeUnit *ComputeUnit) {
	vMemDecode := NewSimpleDecodeUnit(b.CUName+".vmem_decode", b.Engine)
	vMemDecode.Latency = 1
	vMemDecode.Freq = b.Freq
	computeUnit.VMemDecode = vMemDecode

	scalarDecode := NewSimpleDecodeUnit(b.CUName+".scalar_decode", b.Engine)
	scalarDecode.Latency = 1
	scalarDecode.Freq = b.Freq
	computeUnit.ScalarDecode = scalarDecode

	ldsDecode := NewSimpleDecodeUnit(b.CUName+".lds_decode", b.Engine)
	ldsDecode.Latency = 1
	ldsDecode.Freq = b.Freq
	computeUnit.LDSDecode = ldsDecode

	vectorDecode := NewVectorDecodeUnit(b.CUName+".vector_decode", b.Engine)
	vectorDecode.Latency = 1
	vectorDecode.Freq = b.Freq
	computeUnit.VectorDecode = vectorDecode
}

func (b *Builder) initExecUnits(computeUnit *ComputeUnit) {
	for i := 0; i < b.SIMDCount; i++ {
		computeUnit.SIMDUnits = append(computeUnit.SIMDUnits, NewSIMDUnit(
			fmt.Sprintf("%s.%s%d", b.CUName, "simd", i)))
	}

	branchUnit := NewBranchUnit(b.CUName+".branch_unit",
		b.Engine, computeUnit.Scheduler)
	branchUnit.Freq = b.Freq
	computeUnit.BranchUnit = branchUnit

	scalarUnit := NewScalarUnit(b.CUName+".scalar_unit", b.Engine, computeUnit.Scheduler)
	scalarUnit.Freq = b.Freq
	computeUnit.ScalarUnit = scalarUnit

	ldsUnit := NewLDSUnit(b.CUName+".lds_unit", b.Engine, computeUnit.Scheduler)
	ldsUnit.Freq = b.Freq
	computeUnit.LDSUnit = ldsUnit

	computeUnit.VMemUnit = NewVMemUnit(b.CUName + ".vmem_unit")
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

func (b *Builder) setUpDependency(computeUnit *ComputeUnit) {
	scheduler := computeUnit.Scheduler.(*Scheduler)
	scheduler.LDSDecoder = computeUnit.LDSDecode
	scheduler.ScalarDecoder = computeUnit.ScalarDecode
	scheduler.VectorDecoder = computeUnit.VectorDecode
	scheduler.VectorMemDecoder = computeUnit.VMemDecode
	scheduler.BranchUnit = computeUnit.BranchUnit

	vectorDecode := computeUnit.VectorDecode.(*VectorDecodeUnit)
	vectorDecode.SIMDUnits = append(vectorDecode.SIMDUnits,
		computeUnit.SIMDUnits...)

	scalarDecode := computeUnit.ScalarDecode.(*SimpleDecodeUnit)
	scalarDecode.ExecUnit = computeUnit.ScalarUnit

	vMemDecode := computeUnit.VMemDecode.(*SimpleDecodeUnit)
	vMemDecode.ExecUnit = computeUnit.VMemUnit

	ldsDecode := computeUnit.LDSDecode.(*SimpleDecodeUnit)
	ldsDecode.ExecUnit = computeUnit.LDSUnit
}

// connect uses a direct connection to connect all the internal component of
// the compute unit.
//
// Since direct connection is the default connection to use, no latency is
// considered. However, users can overwrite this function to use other type of
// connections inside the compute unit
func (b *Builder) connect(computeUnit *ComputeUnit) {
	connection := core.NewDirectConnection(b.Engine)
	core.PlugIn(computeUnit.Scheduler, "ToSReg", connection)
	core.PlugIn(computeUnit.Scheduler, "ToVRegs", connection)
	core.PlugIn(computeUnit.Scheduler, "ToDecoders", connection)
	core.PlugIn(computeUnit.Scheduler, "FromExecUnits", connection)

	for i := 0; i < b.SIMDCount; i++ {
		core.PlugIn(computeUnit.VRegFiles[i], "ToOutside", connection)
	}
	core.PlugIn(computeUnit.SRegFile, "ToOutside", connection)

	// Decode Units
	core.PlugIn(computeUnit.VMemDecode, "ToExecUnit", connection)
	core.PlugIn(computeUnit.VMemDecode, "FromScheduler", connection)
	core.PlugIn(computeUnit.VectorDecode, "ToExecUnit", connection)
	core.PlugIn(computeUnit.VectorDecode, "FromScheduler", connection)
	core.PlugIn(computeUnit.ScalarDecode, "ToExecUnit", connection)
	core.PlugIn(computeUnit.ScalarDecode, "FromScheduler", connection)
	core.PlugIn(computeUnit.LDSDecode, "ToExecUnit", connection)
	core.PlugIn(computeUnit.LDSDecode, "FromScheduler", connection)

	// Execution Units
	core.PlugIn(computeUnit.BranchUnit, "ToScheduler", connection)
	core.PlugIn(computeUnit.ScalarUnit, "FromDecoder", connection)
	core.PlugIn(computeUnit.ScalarUnit, "ToScheduler", connection)
	core.PlugIn(computeUnit.VMemUnit, "FromDecoder", connection)
	core.PlugIn(computeUnit.VMemUnit, "ToScheduler", connection)
	core.PlugIn(computeUnit.LDSUnit, "FromDecoder", connection)
	core.PlugIn(computeUnit.LDSUnit, "ToScheduler", connection)

	for i := 0; i < b.SIMDCount; i++ {
		core.PlugIn(computeUnit.SIMDUnits[i], "FromDecoder", connection)
		core.PlugIn(computeUnit.SIMDUnits[i], "ToScheduler", connection)
	}

	// External
	core.PlugIn(computeUnit.Scheduler, "ToInstMem", b.ToInstMem)
}
