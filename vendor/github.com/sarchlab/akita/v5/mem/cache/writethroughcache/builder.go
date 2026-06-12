package writethroughcache

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/tracing"
)

// DefaultSpec provides default configuration for the writethroughcache.
var DefaultSpec = Spec{
	Freq:                  1 * sim.GHz,
	NumReqPerCycle:        4,
	Log2BlockSize:         6,
	BankLatency:           20,
	WayAssociativity:      4,
	MaxNumConcurrentTrans: 16,
	NumBanks:              1,
	NumMSHREntry:          4,
	TotalByteSize:         4 * mem.KB,
	DirLatency:            2,
	InterleavingSize:      4096,
}

// A Builder can build a writethroughcache cache
type Builder struct {
	engine                sim.EventScheduler
	spec                  Spec
	log2BlockSize         uint64
	totalByteSize         uint64
	wayAssociativity      int
	numMSHREntry          int
	numBank               int
	dirLatency            int
	bankLatency           int
	numReqPerCycle        int
	maxNumConcurrentTrans int
	visTracer             tracing.Tracer
	missFillExtraLatency  int

	addressMapperType string
	remotePorts       []sim.RemotePort
	interleavingSize  uint64
	legacyMapper      mem.AddressToPortMapper // set by WithAddressToPortMapper, read at Build time
	topPort           sim.Port
	bottomPort        sim.Port
	controlPort       sim.Port
}

// MakeBuilder creates a builder with default parameter setting.
// The default write policy type is "write-around".
func MakeBuilder() Builder {
	return Builder{
		spec:                  DefaultSpec,
		log2BlockSize:         DefaultSpec.Log2BlockSize,
		totalByteSize:         DefaultSpec.TotalByteSize,
		wayAssociativity:      DefaultSpec.WayAssociativity,
		numMSHREntry:          DefaultSpec.NumMSHREntry,
		numBank:               DefaultSpec.NumBanks,
		numReqPerCycle:        DefaultSpec.NumReqPerCycle,
		maxNumConcurrentTrans: DefaultSpec.MaxNumConcurrentTrans,
		dirLatency:            DefaultSpec.DirLatency,
		bankLatency:           DefaultSpec.BankLatency,
		interleavingSize:      DefaultSpec.InterleavingSize,
	}
}

// WithEngine sets the event driven simulation engine that the cache uses
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the cache works at
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithWayAssociativity sets the way associativity the builder builds.
func (b Builder) WithWayAssociativity(wayAssociativity int) Builder {
	b.wayAssociativity = wayAssociativity
	return b
}

// WithNumMSHREntry sets the number of mshr entry
func (b Builder) WithNumMSHREntry(num int) Builder {
	b.numMSHREntry = num
	return b
}

// WithLog2BlockSize sets the number of bytes in a cache line as a power of 2
func (b Builder) WithLog2BlockSize(n uint64) Builder {
	b.log2BlockSize = n
	return b
}

// WithTotalByteSize sets the capacity of the cache unit
func (b Builder) WithTotalByteSize(byteSize uint64) Builder {
	b.totalByteSize = byteSize
	return b
}

// WithNumBanks sets the number of banks in each cache
func (b Builder) WithNumBanks(n int) Builder {
	b.numBank = n
	return b
}

// WithDirectoryLatency sets the number of cycles required to access the
// directory.
func (b Builder) WithDirectoryLatency(n int) Builder {
	b.dirLatency = n
	return b
}

// WithBankLatency sets the number of cycles needed to read to write a
// cacheline.
func (b Builder) WithBankLatency(n int) Builder {
	b.bankLatency = n
	return b
}

// WithMaxNumConcurrentTrans sets the maximum number of concurrent transactions
// that the cache can process.
func (b Builder) WithMaxNumConcurrentTrans(n int) Builder {
	b.maxNumConcurrentTrans = n
	return b
}

// WithNumReqsPerCycle sets the number of requests that the cache can process
// per cycle
func (b Builder) WithNumReqsPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithVisTracer sets the visualization tracer
func (b Builder) WithVisTracer(tracer tracing.Tracer) Builder {
	b.visTracer = tracer
	return b
}

// WithMissFillExtraLatency sets the number of extra cycles added to
// miss-fill read transactions after they exit the bank pipeline. This
// increases the cost of cache misses without affecting hits.
func (b Builder) WithMissFillExtraLatency(n int) Builder {
	b.missFillExtraLatency = n
	return b
}

// WithAddressMapperType sets the type of address mapper to use
func (b Builder) WithAddressMapperType(t string) Builder {
	b.addressMapperType = t
	return b
}

// WithRemotePorts sets the remote ports that the cache can use to send
// requests to other components.
func (b Builder) WithRemotePorts(ports ...sim.RemotePort) Builder {
	b.remotePorts = ports
	return b
}

// WithInterleavingSize sets the interleaving size for the address mapper.
func (b Builder) WithInterleavingSize(size uint64) Builder {
	b.interleavingSize = size
	return b
}

// WithTopPort sets the top port for the cache
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithBottomPort sets the bottom port for the cache
func (b Builder) WithBottomPort(port sim.Port) Builder {
	b.bottomPort = port
	return b
}

// WithControlPort sets the control port for the cache
func (b Builder) WithControlPort(port sim.Port) Builder {
	b.controlPort = port
	return b
}

// WithWritePolicyType sets the write policy type for the cache.
// Valid values: "write-around" (default), "write-evict", "write-through".
func (b Builder) WithWritePolicyType(policyType string) Builder {
	b.spec.WritePolicyType = policyType
	return b
}

// WithAddressToPortMapper specifies how the cache units to create should find
// low level modules. This configures the address mapper using the legacy
// interface. Prefer WithAddressMapperType + WithRemotePorts for new code.
// The mapper is read at Build() time, so its fields can be set after this call.
func (b Builder) WithAddressToPortMapper(
	mapper mem.AddressToPortMapper,
) Builder {
	b.legacyMapper = mapper
	return b
}

// Build returns a new cache component
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	b.assertAllRequiredInformationIsAvailable()
	b.resolveLegacyMapper()

	blockSize := 1 << b.log2BlockSize
	numSets := int(b.totalByteSize / uint64(b.wayAssociativity*blockSize))

	spec := b.buildSpec(numSets)

	initialState := b.buildInitialState(name, numSets, blockSize)

	comp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	comp.SetState(initialState)

	pmw := b.buildPipelineMW(comp)
	b.buildStages(pmw)

	cmw := b.buildControlMW(comp, pmw)

	if b.visTracer != nil {
		tracing.CollectTrace(comp, b.visTracer)
	}

	comp.AddMiddleware(pmw) // index 0
	comp.AddMiddleware(cmw) // index 1

	return comp
}

func (b *Builder) buildInitialState(
	name string,
	numSets, blockSize int,
) State {
	bankBufs := make([]queueing.Buffer[int], b.numBank)
	for i := 0; i < b.numBank; i++ {
		bankBufs[i] = queueing.Buffer[int]{
			BufferName: fmt.Sprintf("%s.Bank%d.Buffer", name, i),
			Cap:        b.maxNumConcurrentTrans,
		}
	}

	bankPipelines := make([]queueing.Pipeline[int], b.numBank)
	for i := 0; i < b.numBank; i++ {
		bankPipelines[i] = queueing.Pipeline[int]{
			Width:     b.numReqPerCycle,
			NumStages: b.bankLatency,
		}
	}

	bankPostBufs := make([]queueing.Buffer[int], b.numBank)
	for i := 0; i < b.numBank; i++ {
		bankPostBufs[i] = queueing.Buffer[int]{
			BufferName: fmt.Sprintf("%s.Bank[%d].PostPipelineBuffer", name, i),
			Cap:        b.numReqPerCycle,
		}
	}

	initialState := State{
		DirBuf: queueing.Buffer[int]{
			BufferName: name + ".DirectoryBuffer",
			Cap:        b.numReqPerCycle,
		},
		BankBufs: bankBufs,
		DirPipeline: queueing.Pipeline[int]{
			Width:     b.numReqPerCycle,
			NumStages: b.dirLatency,
		},
		DirPostBuf: queueing.Buffer[int]{
			BufferName: name + ".DirectoryStage.PostPipelineBuffer",
			Cap:        b.numReqPerCycle,
		},
		BankPipelines: bankPipelines,
		BankPostBufs:  bankPostBufs,
	}

	cache.DirectoryReset(
		&initialState.DirectoryState, numSets, b.wayAssociativity, blockSize)

	return initialState
}

func (b *Builder) buildSpec(numSets int) Spec {
	writePolicyType := b.spec.WritePolicyType
	if writePolicyType == "" {
		writePolicyType = "write-around"
	}

	spec := Spec{
		Freq:                  b.spec.Freq,
		NumReqPerCycle:        b.numReqPerCycle,
		Log2BlockSize:         b.log2BlockSize,
		BankLatency:           b.bankLatency,
		WayAssociativity:      b.wayAssociativity,
		MaxNumConcurrentTrans: b.maxNumConcurrentTrans,
		NumBanks:              b.numBank,
		NumMSHREntry:          b.numMSHREntry,
		NumSets:               numSets,
		TotalByteSize:         b.totalByteSize,
		DirLatency:            b.dirLatency,
		MissFillExtraLatency:  b.missFillExtraLatency,
		WritePolicyType:       writePolicyType,
	}

	if b.addressMapperType != "" {
		remotePortNames := make([]string, len(b.remotePorts))
		for i, rp := range b.remotePorts {
			remotePortNames[i] = string(rp)
		}
		spec.AddressMapperType = b.addressMapperType
		spec.RemotePortNames = remotePortNames
		spec.InterleavingSize = b.interleavingSize
	}

	return spec
}

func (b *Builder) buildPipelineMW(
	comp *modeling.Component[Spec, State],
) *pipelineMW {
	m := &pipelineMW{
		comp: comp,
	}

	m.topPort = b.topPort
	m.topPort.SetComponent(comp)
	comp.AddPort("Top", m.topPort)
	m.bottomPort = b.bottomPort
	m.bottomPort.SetComponent(comp)
	comp.AddPort("Bottom", m.bottomPort)

	m.storage = mem.NewStorage(b.totalByteSize)

	return m
}

func (b *Builder) buildControlMW(
	comp *modeling.Component[Spec, State],
	pmw *pipelineMW,
) *controlMW {
	controlPort := b.controlPort
	controlPort.SetComponent(comp)
	comp.AddPort("Control", controlPort)

	cs := &controlStage{
		ctrlPort:   controlPort,
		pipeline:   pmw,
		bankStages: pmw.bankStages,
	}

	cmw := &controlMW{
		comp:         comp,
		controlStage: cs,
	}

	return cmw
}

func (b *Builder) buildStages(m *pipelineMW) {
	m.intakeStage = &intake{cache: m}
	m.directoryStage = &directory{
		cache: m,
	}
	b.buildBankStages(m)
	m.parseBottomStage = &bottomParser{cache: m}
	m.respondStage = &respondStage{cache: m}
}

func (b *Builder) buildBankStages(m *pipelineMW) {
	for i := 0; i < b.numBank; i++ {
		bs := &bankStage{
			cache:          m,
			bankID:         i,
			numReqPerCycle: b.numReqPerCycle,
		}
		m.bankStages = append(m.bankStages, bs)
	}
}

func (b *Builder) resolveLegacyMapper() {
	if b.legacyMapper == nil {
		return
	}

	switch m := b.legacyMapper.(type) {
	case *mem.SinglePortMapper:
		b.addressMapperType = "single"
		b.remotePorts = []sim.RemotePort{m.Port}
	case *mem.InterleavedAddressPortMapper:
		b.addressMapperType = "interleaved"
		b.remotePorts = m.LowModules
		b.interleavingSize = m.InterleavingSize
	default:
		panic(fmt.Sprintf("unsupported address mapper type: %T", b.legacyMapper))
	}
}

func (b *Builder) assertAllRequiredInformationIsAvailable() {
	if b.engine == nil {
		panic("engine is not specified")
	}
}
