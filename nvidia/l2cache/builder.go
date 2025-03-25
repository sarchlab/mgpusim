package l2cache

import (
	// "fmt"
	"github.com/sarchlab/akita/v4/sim"
	// "github.com/sarchlab/akita/v4/tracing"
)

type L2CacheBuilder struct {
	engine    sim.Engine
	freq      int
	log2Block int
	wayAssoc  int
	totalSize int
	numBanks  int
}

func NewCacheBuilder() *L2CacheBuilder {
	return &L2CacheBuilder{}
}

func (b *L2CacheBuilder) WithEngine(engine sim.Engine) *L2CacheBuilder {
	b.engine = engine
	return b
}

func (b *L2CacheBuilder) WithFreq(freq int) *L2CacheBuilder {
	b.freq = freq
	return b
}

func (b *L2CacheBuilder) WithLog2BlockSize(log2Block int) *L2CacheBuilder {
	b.log2Block = log2Block
	return b
}

func (b *L2CacheBuilder) Build(name string) *L2Cache {
	return NewL1Cache(name, b.engine, b.freq, b.log2Block, b.wayAssoc, b.totalSize, b.numBanks)
}
