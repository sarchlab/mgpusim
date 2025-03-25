package l1cache

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type L1Cache struct {
	Name      string
	engine    sim.Engine
	freq      int
	log2Block int
	wayAssoc  int
	totalSize int
	numBanks  int
	visTracer tracing.Tracer
	memTracer tracing.Tracer
}

func NewL1Cache(name string, engine sim.Engine, freq int, log2Block int, wayAssoc int, totalSize int, numBanks int) *L1Cache {
	return &L1Cache{
		Name:      name,
		engine:    engine,
		freq:      freq,
		log2Block: log2Block,
		wayAssoc:  wayAssoc,
		totalSize: totalSize,
		numBanks:  numBanks,
	}
}

func (c *L1Cache) Build() {
	fmt.Printf("Building L1 Cache: %s\n", c.Name)
	// Add logic to initialize cache components
}
