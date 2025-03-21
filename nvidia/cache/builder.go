package cache

import (
    // "fmt"
    "github.com/sarchlab/akita/v4/sim"
    // "github.com/sarchlab/akita/v4/tracing"
)

type CacheBuilder struct {
    engine     sim.Engine
    freq       int
    log2Block  int
    wayAssoc   int
    totalSize  int
    numBanks   int
}

func NewCacheBuilder() *CacheBuilder {
    return &CacheBuilder{}
}

func (b *CacheBuilder) WithEngine(engine sim.Engine) *CacheBuilder {
    b.engine = engine
    return b
}

func (b *CacheBuilder) WithFreq(freq int) *CacheBuilder {
    b.freq = freq
    return b
}

func (b *CacheBuilder) WithLog2BlockSize(log2Block int) *CacheBuilder {
    b.log2Block = log2Block
    return b
}

func (b *CacheBuilder) Build(name string) *L1Cache {
    return NewL1Cache(name, b.engine, b.freq, b.log2Block, b.wayAssoc, b.totalSize, b.numBanks)
}