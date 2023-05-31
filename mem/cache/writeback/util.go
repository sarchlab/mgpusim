package writeback

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
)

func getCacheLineID(
	addr uint64,
	blockSizeAsPowerOf2 uint64,
) (cacheLineID, offset uint64) {
	mask := uint64(0xffffffffffffffff << blockSizeAsPowerOf2)
	cacheLineID = addr & mask
	offset = addr & ^mask
	return
}

func bankID(block *cache.Block, wayAssocitivity, numBanks int) int {
	return (block.SetID*wayAssocitivity + block.WayID) % numBanks
}

func clearPort(p sim.Port, now sim.VTimeInSec) {
	for {
		item := p.Retrieve(now)
		if item == nil {
			return
		}
	}
}
