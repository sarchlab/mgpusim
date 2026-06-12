package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/queueing"
)

// resolveLegacyMapper converts a legacy AddressToPortMapper set via
// WithAddressToPortMapper into the builder's addressMapperType/remotePorts/
// interleavingSize fields. This allows Build() to always populate Spec from
// the builder.
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

// DefaultSpec provides default configuration for the writeback cache.
var DefaultSpec = Spec{
	Freq:                1 * sim.GHz,
	NumReqPerCycle:      1,
	Log2BlockSize:       6,
	BankLatency:         10,
	WayAssociativity:    4,
	NumBanks:            1,
	NumMSHREntry:        16,
	TotalByteSize:       512 * mem.KB,
	WriteBufferCapacity: 1024,
	MaxInflightFetch:    128,
	MaxInflightEviction: 128,
	InterleavingSize:    4096,
}

// A Builder can build writeback caches
type Builder struct {
	engine           sim.EventScheduler
	spec             Spec
	legacyMapper     mem.AddressToPortMapper
	wayAssociativity int
	log2BlockSize    uint64

	byteSize            uint64
	numMSHREntry        int
	numReqPerCycle      int
	writeBufferCapacity int
	maxInflightFetch    int
	maxInflightEviction int

	dirLatency  int
	bankLatency int

	addressMapperType string
	remotePorts       []sim.RemotePort
	interleavingSize  uint64

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port
}

// MakeBuilder creates a new builder with default configurations.
func MakeBuilder() Builder {
	return Builder{
		spec:                DefaultSpec,
		wayAssociativity:    DefaultSpec.WayAssociativity,
		log2BlockSize:       DefaultSpec.Log2BlockSize,
		byteSize:            DefaultSpec.TotalByteSize,
		numMSHREntry:        DefaultSpec.NumMSHREntry,
		numReqPerCycle:      DefaultSpec.NumReqPerCycle,
		writeBufferCapacity: DefaultSpec.WriteBufferCapacity,
		maxInflightFetch:    DefaultSpec.MaxInflightFetch,
		maxInflightEviction: DefaultSpec.MaxInflightEviction,
		bankLatency:         DefaultSpec.BankLatency,
		interleavingSize:    DefaultSpec.InterleavingSize,
	}
}

// WithEngine sets the engine to be used by the caches.
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency to be used by the caches.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithWayAssociativity sets the way associativity.
func (b Builder) WithWayAssociativity(n int) Builder {
	b.wayAssociativity = n
	return b
}

// WithLog2BlockSize sets the cache line size as the power of 2.
func (b Builder) WithLog2BlockSize(n uint64) Builder {
	b.log2BlockSize = n
	return b
}

// WithNumMSHREntry sets the number of MSHR entries.
func (b Builder) WithNumMSHREntry(n int) Builder {
	b.numMSHREntry = n
	return b
}

// WithAddressToPortMapper sets the AddressToPortMapper to be used.
// The mapper is read lazily at Tick time, so its fields can be set after Build.
func (b Builder) WithAddressToPortMapper(f mem.AddressToPortMapper) Builder {
	b.legacyMapper = f
	return b
}

// WithNumReqPerCycle sets the number of requests that can be processed by the
// cache in each cycle.
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithByteSize set the size of the cache.
func (b Builder) WithByteSize(byteSize uint64) Builder {
	b.byteSize = byteSize
	return b
}

// WithWriteBufferSize sets the number of cache lines that can reside in the
// write buffer.
func (b Builder) WithWriteBufferSize(n int) Builder {
	b.writeBufferCapacity = n
	return b
}

// WithMaxInflightFetch sets the number of concurrent fetch that the write-back
// cache can issue at the same time.
func (b Builder) WithMaxInflightFetch(n int) Builder {
	b.maxInflightFetch = n
	return b
}

// WithMaxInflightEviction sets the number of concurrent eviction that the
// write buffer can write to a low-level module.
func (b Builder) WithMaxInflightEviction(n int) Builder {
	b.maxInflightEviction = n
	return b
}

// WithDirectoryLatency sets the number of cycles required to access the
// directory.
func (b Builder) WithDirectoryLatency(n int) Builder {
	b.dirLatency = n
	return b
}

// WithBankLatency sets the number of cycles required to process each cache
// read/write operation.
func (b Builder) WithBankLatency(n int) Builder {
	b.bankLatency = n
	return b
}

// WithAddressMapperType sets the address mapper type.
func (b Builder) WithAddressMapperType(t string) Builder {
	b.addressMapperType = t
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

// WithRemotePorts sets the remote ports for address mapping.
func (b Builder) WithRemotePorts(ports ...sim.RemotePort) Builder {
	b.remotePorts = ports
	return b
}

// WithInterleavingSize sets the interleaving size for the address mapper.
func (b Builder) WithInterleavingSize(size uint64) Builder {
	b.interleavingSize = size
	return b
}

// Build creates a usable writeback cache.
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	b.resolveLegacyMapper()

	blockSize := 1 << b.log2BlockSize
	numSets := int(b.byteSize / uint64(b.wayAssociativity*blockSize))

	spec := b.buildSpec(numSets)

	laneWidth := b.numReqPerCycle
	if laneWidth == 1 {
		laneWidth = 2
	}

	initialState := b.buildInitialState(name, laneWidth, numSets)

	comp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	comp.SetState(initialState)

	pmw := b.buildPipelineMW(comp, laneWidth)
	cmw := b.buildControlMW(comp, pmw)

	comp.AddMiddleware(pmw) // index 0
	comp.AddMiddleware(cmw) // index 1

	return comp
}

func (b Builder) buildInitialState(
	name string, laneWidth, numSets int,
) State {
	blockSize := 1 << b.log2BlockSize

	s := State{
		CacheState:   int(cacheStateRunning),
		EvictingList: make(map[uint64]bool),
		DirStageBuf: queueing.Buffer[int]{
			BufferName: name + ".DirStageBuf",
			Cap:        b.numReqPerCycle,
		},
		DirToBankBufs: []queueing.Buffer[int]{{
			BufferName: name + ".DirToBankBuf",
			Cap:        b.numReqPerCycle,
		}},
		WriteBufferToBankBufs: []queueing.Buffer[int]{{
			BufferName: name + ".WriteBufferToBankBuf",
			Cap:        b.numReqPerCycle,
		}},
		MSHRStageBuf: queueing.Buffer[int]{
			BufferName: name + ".MSHRStageBuf",
			Cap:        b.numReqPerCycle,
		},
		WriteBufferBuf: queueing.Buffer[int]{
			BufferName: name + ".WriteBufferBuf",
			Cap:        b.numReqPerCycle,
		},
		DirPipeline: queueing.Pipeline[int]{
			Width:     laneWidth,
			NumStages: b.dirLatency,
		},
		DirPostPipelineBuf: queueing.Buffer[int]{
			BufferName: name + ".DirPostPipelineBuf",
			Cap:        b.numReqPerCycle,
		},
		BankPipelines: []queueing.Pipeline[int]{{
			Width:     laneWidth,
			NumStages: b.bankLatency,
		}},
		BankPostPipelineBufs: []queueing.Buffer[int]{{
			BufferName: name + ".BankPostPipelineBuf",
			Cap:        laneWidth,
		}},
		BankInflightTransCounts:         make([]int, 1),
		BankDownwardInflightTransCounts: make([]int, 1),
	}

	cache.DirectoryReset(
		&s.DirectoryState, numSets, b.wayAssociativity, blockSize)

	return s
}

func (b *Builder) buildSpec(numSets int) Spec {
	spec := Spec{
		Freq:                b.spec.Freq,
		NumReqPerCycle:      b.numReqPerCycle,
		Log2BlockSize:       b.log2BlockSize,
		BankLatency:         b.bankLatency,
		WayAssociativity:    b.wayAssociativity,
		NumBanks:            1,
		NumSets:             numSets,
		NumMSHREntry:        b.numMSHREntry,
		TotalByteSize:       b.byteSize,
		DirLatency:          b.dirLatency,
		WriteBufferCapacity: b.writeBufferCapacity,
		MaxInflightFetch:    b.maxInflightFetch,
		MaxInflightEviction: b.maxInflightEviction,
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
	laneWidth int,
) *pipelineMW {
	m := &pipelineMW{
		comp: comp,
	}

	b.createPipelinePorts(m, comp)
	m.storage = mem.NewStorage(b.byteSize)

	b.createInternalStages(m, laneWidth)

	return m
}

func (b *Builder) buildControlMW(
	comp *modeling.Component[Spec, State],
	pmw *pipelineMW,
) *controlMW {
	controlPort := b.controlPort
	controlPort.SetComponent(comp)
	comp.AddPort("Control", controlPort)

	f := &flusher{
		pipeline: pmw,
		ctrlPort: controlPort,
	}

	cmw := &controlMW{
		comp:    comp,
		flusher: f,
	}

	return cmw
}

func (b *Builder) createPipelinePorts(
	m *pipelineMW,
	comp *modeling.Component[Spec, State],
) {
	m.topPort = b.topPort
	m.topPort.SetComponent(comp)
	comp.AddPort("Top", m.topPort)

	m.bottomPort = b.bottomPort
	m.bottomPort.SetComponent(comp)
	comp.AddPort("Bottom", m.bottomPort)
}

func (b *Builder) createInternalStages(m *pipelineMW, laneWidth int) {
	m.topParser = &topParser{cache: m}

	m.dirStage = &directoryStage{
		cache: m,
	}

	m.bankStages = make([]*bankStage, 1)
	m.bankStages[0] = &bankStage{
		cache:         m,
		bankID:        0,
		pipelineWidth: laneWidth,
	}

	m.mshrStage = &mshrStage{cache: m}
	m.writeBuffer = &writeBufferStage{
		cache: m,
	}
}
