package l1cache

import (
	// "fmt"
	"github.com/sarchlab/akita/v4/sim"
	// "github.com/sarchlab/akita/v4/tracing"
)

type L1CacheBuilder struct {
	engine    sim.Engine
	freq      int
	log2Block int
	wayAssoc  int
	totalSize int
	numBanks  int
}

func NewCacheBuilder() *L1CacheBuilder {
	return &L1CacheBuilder{}
}

func (b *L1CacheBuilder) WithEngine(engine sim.Engine) *L1CacheBuilder {
	b.engine = engine
	return b
}

func (b *L1CacheBuilder) WithFreq(freq int) *L1CacheBuilder {
	b.freq = freq
	return b
}

func (b *L1CacheBuilder) WithLog2BlockSize(log2Block int) *L1CacheBuilder {
	b.log2Block = log2Block
	return b
}

func (b *L1CacheBuilder) Build(name string) *L1Cache {
	return NewL1Cache(name, b.engine, b.freq, b.log2Block, b.wayAssoc, b.totalSize, b.numBanks)
}
