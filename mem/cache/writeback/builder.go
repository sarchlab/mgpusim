package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v3/pipelining"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

// A Builder can build writeback caches
type Builder struct {
	engine           sim.Engine
	freq             sim.Freq
	lowModuleFinder  mem.LowModuleFinder
	wayAssociativity int
	log2BlockSize    uint64

	interleaving          bool
	numInterleavingBlock  int
	interleavingUnitCount int
	interleavingUnitIndex int

	byteSize            uint64
	numMSHREntry        int
	numReqPerCycle      int
	writeBufferCapacity int
	maxInflightFetch    int
	maxInflightEviction int

	dirLatency  int
	bankLatency int
}

// MakeBuilder creates a new builder with default configurations.
func MakeBuilder() Builder {
	return Builder{
		freq:                1 * sim.GHz,
		wayAssociativity:    4,
		log2BlockSize:       6,
		byteSize:            512 * mem.KB,
		numMSHREntry:        16,
		numReqPerCycle:      1,
		writeBufferCapacity: 1024,
		maxInflightFetch:    128,
		maxInflightEviction: 128,
		bankLatency:         10,
	}
}

// WithEngine sets the engine to be used by the caches.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency to be used by the caches.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
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

// WithLowModuleFinder sets the LowModuleFinder to be used.
func (b Builder) WithLowModuleFinder(f mem.LowModuleFinder) Builder {
	b.lowModuleFinder = f
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

// WithInterleaving sets the size that the cache is interleaved.
func (b Builder) WithInterleaving(
	numBlock, unitCount, unitIndex int,
) Builder {
	b.interleaving = true
	b.numInterleavingBlock = numBlock
	b.interleavingUnitCount = unitCount
	b.interleavingUnitIndex = unitIndex
	return b
}

// WithWriteBufferSize sets the number of cach lines that can reside in the
// writebuffer.
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

// WithBankLatency sets the number of cycles required to process each can
// read/write operation.
func (b Builder) WithBankLatency(n int) Builder {
	b.bankLatency = n
	return b
}

// Build creates a usable writeback cache.
func (b Builder) Build(name string) *Cache {
	cache := new(Cache)
	cache.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, cache)

	b.configureCache(cache)
	b.createPorts(cache)
	b.createPortSenders(cache)
	b.createInternalStages(cache)
	b.createInternalBuffers(cache)

	return cache
}

func (b *Builder) configureCache(cacheModule *Cache) {
	blockSize := 1 << b.log2BlockSize
	vimctimFinder := cache.NewLRUVictimFinder()
	numSet := int(b.byteSize / uint64(b.wayAssociativity*blockSize))
	directory := cache.NewDirectory(
		numSet, b.wayAssociativity, blockSize, vimctimFinder)

	if b.interleaving {
		directory.AddrConverter = &mem.InterleavingConverter{
			InterleavingSize:    uint64(b.numInterleavingBlock) * (1 << b.log2BlockSize),
			TotalNumOfElements:  b.interleavingUnitCount,
			CurrentElementIndex: b.interleavingUnitIndex,
		}
	}

	mshr := cache.NewMSHR(b.numMSHREntry)
	storage := mem.NewStorage(b.byteSize)

	cacheModule.log2BlockSize = b.log2BlockSize
	cacheModule.numReqPerCycle = b.numReqPerCycle
	cacheModule.directory = directory
	cacheModule.mshr = mshr
	cacheModule.storage = storage
	cacheModule.lowModuleFinder = b.lowModuleFinder
	cacheModule.state = cacheStateRunning
	cacheModule.evictingList = make(map[uint64]bool)
}

func (b *Builder) createPorts(cache *Cache) {
	cache.topPort = sim.NewLimitNumMsgPort(cache,
		cache.numReqPerCycle*2, cache.Name()+".ToTop")
	cache.AddPort("Top", cache.topPort)

	cache.bottomPort = sim.NewLimitNumMsgPort(cache,
		cache.numReqPerCycle*2, cache.Name()+".BottomPort")
	cache.AddPort("Bottom", cache.bottomPort)

	cache.controlPort = sim.NewLimitNumMsgPort(cache,
		cache.numReqPerCycle*2, cache.Name()+".ControlPort")
	cache.AddPort("Control", cache.controlPort)
}

func (b *Builder) createPortSenders(cache *Cache) {
	cache.topSender = sim.NewBufferedSender(
		cache.topPort,
		sim.NewBuffer(cache.Name()+".TopSenderBuffer",
			cache.numReqPerCycle*4,
		),
	)
	cache.bottomSender = sim.NewBufferedSender(
		cache.bottomPort,
		sim.NewBuffer(
			cache.Name()+".BottomSenderBuffer",
			cache.numReqPerCycle*4,
		),
	)
	cache.controlPortSender = sim.NewBufferedSender(
		cache.controlPort, sim.NewBuffer(
			cache.Name()+".ControlSenderBuffer",
			cache.numReqPerCycle*4,
		),
	)
}

func (b *Builder) createInternalStages(cache *Cache) {
	cache.topParser = &topParser{cache: cache}
	b.buildDirectoryStage(cache)
	b.buildBankStages(cache)
	cache.mshrStage = &mshrStage{cache: cache}
	cache.flusher = &flusher{cache: cache}
	cache.writeBuffer = &writeBufferStage{
		cache:               cache,
		writeBufferCapacity: b.writeBufferCapacity,
		maxInflightFetch:    b.maxInflightFetch,
		maxInflightEviction: b.maxInflightEviction,
	}
}

func (b *Builder) buildDirectoryStage(cache *Cache) {
	buf := sim.NewBuffer(
		cache.Name()+".DirectoryStageBuffer",
		b.numReqPerCycle,
	)
	pipeline := pipelining.
		MakeBuilder().
		WithCyclePerStage(1).
		WithNumStage(b.dirLatency).
		WithPipelineWidth(b.numReqPerCycle).
		WithPostPipelineBuffer(buf).
		Build(cache.Name() + ".BankPipeline")
	cache.dirStage = &directoryStage{
		cache:    cache,
		pipeline: pipeline,
		buf:      buf,
	}
}

func (b *Builder) buildBankStages(cache *Cache) {
	cache.bankStages = make([]*bankStage, 1)

	laneWidth := b.numReqPerCycle
	if laneWidth == 1 {
		laneWidth = 2
	}

	buf := &bufferImpl{
		name:     fmt.Sprintf("%s.Bank.PostPipelineBuffer", cache.Name()),
		capacity: laneWidth,
	}
	pipeline := pipelining.
		MakeBuilder().
		WithCyclePerStage(1).
		WithNumStage(b.bankLatency).
		WithPipelineWidth(laneWidth).
		WithPostPipelineBuffer(buf).
		Build(fmt.Sprintf("%s.Bank.Pipeline", cache.Name()))
	cache.bankStages[0] = &bankStage{
		cache:           cache,
		bankID:          0,
		pipeline:        pipeline,
		postPipelineBuf: buf,
		pipelineWidth:   laneWidth,
	}
}

func (b *Builder) createInternalBuffers(cache *Cache) {
	cache.dirStageBuffer = sim.NewBuffer(
		cache.Name()+".DirStageBuffer",
		cache.numReqPerCycle,
	)
	cache.dirToBankBuffers = make([]sim.Buffer, 1)
	cache.dirToBankBuffers[0] = sim.NewBuffer(
		cache.Name()+".DirToBankBuffer",
		cache.numReqPerCycle,
	)
	cache.writeBufferToBankBuffers = make([]sim.Buffer, 1)
	cache.writeBufferToBankBuffers[0] = sim.NewBuffer(
		cache.Name()+".WriteBufferToBankBuffer",
		cache.numReqPerCycle,
	)
	cache.mshrStageBuffer = sim.NewBuffer(
		cache.Name()+".MSHRStageBuffer",
		cache.numReqPerCycle,
	)
	cache.writeBufferBuffer = sim.NewBuffer(
		cache.Name()+".WriteBufferBuffer",
		cache.numReqPerCycle,
	)
}
