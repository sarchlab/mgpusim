package writearound

import (
	"fmt"

	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mem/v2/cache"
	"gitlab.com/akita/mem/v2/mem"
	"gitlab.com/akita/util/v2/buffering"
	"gitlab.com/akita/util/v2/pipelining"
	"gitlab.com/akita/util/v2/tracing"
)

// A Builder can build an writearound cache
type Builder struct {
	engine          sim.Engine
	freq            sim.Freq
	log2BlockSize   uint64
	totalByteSize   uint64
	wayAssocitivity int
	numMSHREntry    int
	numBank         int
	bankLatency     int
	numReqPerCycle  int
	lowModuleFinder mem.LowModuleFinder
	visTracer       tracing.Tracer
}

// NewBuilder creates a builder with default parameter setting
func NewBuilder() *Builder {
	return &Builder{
		freq:            1 * sim.GHz,
		log2BlockSize:   6,
		totalByteSize:   4 * mem.KB,
		wayAssocitivity: 2,
		numMSHREntry:    4,
		numBank:         1,
		numReqPerCycle:  4,
		bankLatency:     20,
	}
}

// WithEngine sets the event driven simulation engine that the cache uses
func (b *Builder) WithEngine(engine sim.Engine) *Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the cache works at
func (b *Builder) WithFreq(freq sim.Freq) *Builder {
	b.freq = freq
	return b
}

// WithWayAssocitivity sets the way associtivity the builder builds.
func (b *Builder) WithWayAssocitivity(wayAssocitivity int) *Builder {
	b.wayAssocitivity = wayAssocitivity
	return b
}

// WithNumMSHREntry sets the number of mshr entry
func (b *Builder) WithNumMSHREntry(num int) *Builder {
	b.numMSHREntry = num
	return b
}

// WithLog2BlockSize sets the number of bytes in a cache line as a power of 2
func (b *Builder) WithLog2BlockSize(n uint64) *Builder {
	b.log2BlockSize = n
	return b
}

// WithTotalByteSize sets the capacity of the cache unit
func (b *Builder) WithTotalByteSize(byteSize uint64) *Builder {
	b.totalByteSize = byteSize
	return b
}

// WithNumBanks sets the number of banks in each cache
func (b *Builder) WithNumBanks(n int) *Builder {
	b.numBank = n
	return b
}

// WithBankLatency sets the number of cycles needed to read to write a
// cacheline.
func (b *Builder) WithBankLatency(n int) *Builder {
	b.bankLatency = n
	return b
}

// WithNumReqsPerCycle sets the number of requests that the cache can process
// per cycle
func (b *Builder) WithNumReqsPerCycle(n int) *Builder {
	b.numReqPerCycle = n
	return b
}

// WithVisTracer sets the visualization tracer
func (b *Builder) WithVisTracer(tracer tracing.Tracer) *Builder {
	b.visTracer = tracer
	return b
}

// WithLowModuleFinder specifies how the cache units to create should find low
// level modules.
func (b *Builder) WithLowModuleFinder(
	lowModuleFinder mem.LowModuleFinder,
) *Builder {
	b.lowModuleFinder = lowModuleFinder
	return b
}

// Build returns a new cache unit
func (b *Builder) Build(name string) *Cache {
	b.assertAllRequiredInformationIsAvailable()

	c := &Cache{
		log2BlockSize:  b.log2BlockSize,
		numReqPerCycle: b.numReqPerCycle,
	}
	c.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, c)

	c.topPort = sim.NewLimitNumMsgPort(c, b.numReqPerCycle, name+".TopPort")
	c.AddPort("Top", c.topPort)
	c.bottomPort = sim.NewLimitNumMsgPort(c, b.numReqPerCycle,
		name+".BottomPort")
	c.AddPort("Bottom", c.bottomPort)
	c.controlPort = sim.NewLimitNumMsgPort(c, b.numReqPerCycle,
		name+"ControlPort")
	c.AddPort("Control", c.controlPort)

	c.dirBuf = buffering.NewBuffer(b.numReqPerCycle)
	c.bankBufs = make([]buffering.Buffer, b.numBank)
	for i := 0; i < b.numBank; i++ {
		c.bankBufs[i] = buffering.NewBuffer(b.numReqPerCycle)
	}

	c.mshr = cache.NewMSHR(b.numMSHREntry)
	blockSize := 1 << b.log2BlockSize
	numSets := int(b.totalByteSize / uint64(b.wayAssocitivity*blockSize))
	c.directory = cache.NewDirectory(
		numSets, b.wayAssocitivity, 1<<b.log2BlockSize,
		cache.NewLRUVictimFinder())
	c.storage = mem.NewStorage(b.totalByteSize)
	c.bankLatency = b.bankLatency
	c.wayAssociativity = b.wayAssocitivity
	c.lowModuleFinder = b.lowModuleFinder

	b.buildStages(c)

	if b.visTracer != nil {
		tracing.CollectTrace(c, b.visTracer)
	}

	return c
}

func (b *Builder) buildStages(c *Cache) {
	c.coalesceStage = &coalescer{cache: c}
	c.directoryStage = &directory{cache: c}
	for i := 0; i < b.numBank; i++ {
		pipelineName := fmt.Sprintf("%s.Bank_%02d.Pipeline", c.Name(), i)
		postPipelineBuf := buffering.NewBuffer(b.numReqPerCycle)
		pipeline := pipelining.MakeBuilder().
			WithPipelineWidth(b.numReqPerCycle).
			WithNumStage(b.bankLatency).
			WithCyclePerStage(1).
			WithPostPipelineBuffer(postPipelineBuf).
			Build(pipelineName)
		bs := &bankStage{
			cache:           c,
			bankID:          i,
			numReqPerCycle:  b.numReqPerCycle,
			pipeline:        pipeline,
			postPipelineBuf: postPipelineBuf,
		}
		c.bankStages = append(c.bankStages, bs)

		if b.visTracer != nil {
			tracing.CollectTrace(bs.pipeline, b.visTracer)
		}
	}
	c.parseBottomStage = &bottomParser{cache: c}
	c.respondStage = &respondStage{cache: c}

	c.controlStage = &controlStage{
		ctrlPort:     c.controlPort,
		transactions: &c.transactions,
		directory:    c.directory,
		cache:        c,
		bankStages:   c.bankStages,
		coalescer:    c.coalesceStage,
	}
}

func (b *Builder) assertAllRequiredInformationIsAvailable() {
	if b.engine == nil {
		panic("engine is not specified")
	}
}
