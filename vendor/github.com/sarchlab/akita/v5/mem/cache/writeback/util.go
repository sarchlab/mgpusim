package writeback

import (
	"github.com/sarchlab/akita/v5/sim"
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

func bankID(setID, wayID, wayAssociativity, numBanks int) int {
	return (setID*wayAssociativity + wayID) % numBanks
}

func clearPort(p sim.Port) {
	for {
		item := p.RetrieveIncoming()
		if item == nil {
			return
		}
	}
}
