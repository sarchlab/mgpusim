package l1v

import (
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

	c := &Cache{}
	c.TickingComponent = akitaext.NewTickingComponent(
		name, b.engine, b.freq, c)

	c.TopPort = akita.NewLimitNumReqPort(c, 4)
	c.BottomPort = akita.NewLimitNumReqPort(c, 4)
	c.ControlPort = akita.NewLimitNumReqPort(c, 4)

	c.dirBuf = util.NewBuffer(4)
	c.bankBufs = []util.Buffer{util.NewBuffer(4)}

	mshr := cache.NewMSHR(b.numMSHREntry)
	blockSize := 1 << b.log2BlockSize
	numSets := int(b.totalByteSize / uint64(b.wayAssocitivity*blockSize))
	dir := cache.NewDirectory(
		numSets, b.wayAssocitivity, 1<<b.log2BlockSize,
		cache.NewLRUVictimFinder())

	c.coalesceStage = &coalescer{
		topPort:                  c.TopPort,
		dirBuf:                   c.dirBuf,
		transactions:             &c.transactions,
		postCoalesceTransactions: &c.postCoalesceTransactions,
	}

	c.directoryStage = &directory{
		inBuf:           c.dirBuf,
		dir:             dir,
		mshr:            mshr,
		bottomPort:      c.BottomPort,
		bankBufs:        c.bankBufs,
		lowModuleFinder: b.lowModuleFinder,
		log2BlockSize:   b.log2BlockSize,
	}

	c.parseBottomStage = &bottomParser{
		bottomPort:       c.BottomPort,
		mshr:             mshr,
		bankBufs:         c.bankBufs,
		transactions:     &c.postCoalesceTransactions,
		log2BlockSize:    b.log2BlockSize,
		wayAssociativity: b.wayAssocitivity,
	}

	c.respondStage = &respondStage{
		topPort:      c.TopPort,
		transactions: &c.transactions,
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
