package l1v

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
	"gitlab.com/akita/util/akitaext"
)

// A Builder can build an l1v cache
type Builder struct {
	engine          akita.Engine
	freq            akita.Freq
	log2BlockSize   uint64
	totalByteSize   uint64
	wayAssocitivity int
	numMSHREntry    int
	numBank         int
	bankLatency     int
	numReqsPerCycle int
	lowModuleFinder cache.LowModuleFinder
}

// NewBuilder creates a builder with default parameter setting
func NewBuilder() *Builder {
	return &Builder{
		freq:            1 * akita.GHz,
		log2BlockSize:   6,
		totalByteSize:   4 * mem.KB,
		wayAssocitivity: 2,
		numMSHREntry:    4,
		numBank:         1,
		numReqsPerCycle: 4,
		bankLatency:     0,
	}
}

// WithEngine sets the event driven simulation engine that the cache uses
func (b *Builder) WithEngine(engine akita.Engine) *Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the cache works at
func (b *Builder) WithFreq(freq akita.Freq) *Builder {
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
	b.numReqsPerCycle = n
	return b
}

// WithLowModuleFinder specifies how the cache units to create should find low
// level modules.
func (b *Builder) WithLowModuleFinder(
	lowModuleFinder cache.LowModuleFinder,
) *Builder {
	b.lowModuleFinder = lowModuleFinder
	return b
}

// Build returns a new cache unit
func (b *Builder) Build(name string) *Cache {
	b.assertAllRequiredInformationIsAvailable()

	c := &Cache{
		log2BlockSize:  b.log2BlockSize,
		numReqPerCycle: b.numReqsPerCycle,
	}
	c.TickingComponent = akitaext.NewTickingComponent(
		name, b.engine, b.freq, c)

	c.TopPort = akita.NewLimitNumReqPort(c, b.numReqsPerCycle)
	c.BottomPort = akita.NewLimitNumReqPort(c, b.numReqsPerCycle)
	c.ControlPort = akita.NewLimitNumReqPort(c, b.numReqsPerCycle)

	c.dirBuf = util.NewBuffer(b.numReqsPerCycle)
	c.bankBufs = make([]util.Buffer, b.numBank)
	for i := 0; i < b.numBank; i++ {
		c.bankBufs[i] = util.NewBuffer(b.numReqsPerCycle)
	}

	mshr := cache.NewMSHR(b.numMSHREntry)
	blockSize := 1 << b.log2BlockSize
	numSets := int(b.totalByteSize / uint64(b.wayAssocitivity*blockSize))
	dir := cache.NewDirectory(
		numSets, b.wayAssocitivity, 1<<b.log2BlockSize,
		cache.NewLRUVictimFinder())
	storage := mem.NewStorage(b.totalByteSize)

	c.coalesceStage = &coalescer{
		cache: c,
	}

	c.directoryStage = &directory{
		name:            name + ".directory_stage",
		inBuf:           c.dirBuf,
		dir:             dir,
		mshr:            mshr,
		bottomPort:      c.BottomPort,
		bankBufs:        c.bankBufs,
		lowModuleFinder: b.lowModuleFinder,
		log2BlockSize:   b.log2BlockSize,
	}

	for i := 0; i < b.numBank; i++ {
		bs := &bankStage{
			name:              fmt.Sprintf("%s.bank_stage%d", name, i),
			inBuf:             c.bankBufs[i],
			storage:           storage,
			postCTransactions: &c.postCoalesceTransactions,
			latency:           b.bankLatency,
			log2BlockSize:     b.log2BlockSize,
		}
		c.bankStages = append(c.bankStages, bs)
	}

	c.parseBottomStage = &bottomParser{
		name:             name + ".parse_bottom_stage",
		bottomPort:       c.BottomPort,
		mshr:             mshr,
		bankBufs:         c.bankBufs,
		transactions:     &c.postCoalesceTransactions,
		log2BlockSize:    b.log2BlockSize,
		wayAssociativity: b.wayAssocitivity,
	}

	c.respondStage = &respondStage{
		name:         name + ".respond_stage",
		topPort:      c.TopPort,
		transactions: &c.transactions,
	}

	c.controlStage = &controlStage{
		ctrlPort:     c.ControlPort,
		transactions: &c.transactions,
		directory:    dir,
	}

	return c
}

func (b *Builder) assertAllRequiredInformationIsAvailable() {
	if b.engine == nil {
		panic("engine is not specified")
	}

	if b.lowModuleFinder == nil {
		panic("lowModuleFinder is not specified")
	}
}
