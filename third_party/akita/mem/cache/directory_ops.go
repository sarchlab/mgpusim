package cache

import "github.com/sarchlab/akita/v5/mem/vm"

// directorySetID maps a cache-line-aligned address to a set index.
//
// A plain `blockID % numSets` indexes on contiguous low-order block-address
// bits. When an upstream bank/channel interleaver selects the cache slice using
// some of those same low bits, those bits are constant within a slice, so only
// a fraction of the sets are ever reachable and the slice's effective capacity
// collapses (e.g. a 256 KB, 16-way slice behind a 128 B 16-way interleave can
// reach only 16 of its 256 sets -> ~1/16 of capacity). Folding the higher-order
// block-address bits into the index with XOR keeps every set reachable
// regardless of which low bits the interleaver consumed. The stored tag is the
// full address, so lookups remain correct; only the set distribution changes.
//
// NOTE: vendored from akita with this fix; to be upstreamed.
func directorySetID(addr uint64, blockSize, numSets int) int {
	blockID := addr / uint64(blockSize)
	blockID ^= blockID >> 20
	blockID ^= blockID >> 10
	return int(blockID % uint64(numSets))
}

// DirectoryLookup finds the block matching pid+addr in DirectoryState.
// addr should already be cache-line aligned.
// Returns (setID, wayID, found).
func DirectoryLookup(
	ds *DirectoryState,
	numSets int,
	blockSize int,
	pid vm.PID,
	addr uint64,
) (int, int, bool) {
	setID := directorySetID(addr, blockSize, numSets)
	set := &ds.Sets[setID]

	for wayID, block := range set.Blocks {
		if block.IsValid && block.Tag == addr && vm.PID(block.PID) == pid {
			return setID, wayID, true
		}
	}

	return setID, -1, false
}

// DirectoryFindVictim returns (setID, wayID) of the best eviction victim
// for addr. Ways are scanned in LRU order; the first one that is neither
// IsLocked nor has outstanding readers (ReadCount > 0) is returned. When
// every way in the set is busy, LRUOrder[0] is returned so the caller's
// own IsLocked/ReadCount guard can decide to stall — preserving the
// previous behavior on that pathological case.
func DirectoryFindVictim(
	ds *DirectoryState,
	numSets int,
	blockSize int,
	addr uint64,
) (int, int) {
	setID := directorySetID(addr, blockSize, numSets)
	set := &ds.Sets[setID]

	for _, wayID := range set.LRUOrder {
		block := &set.Blocks[wayID]
		if !block.IsLocked && block.ReadCount == 0 {
			return setID, wayID
		}
	}

	return setID, set.LRUOrder[0]
}

// DirectoryVisit moves the block at (setID, wayID) to MRU position
// (end of LRUOrder).
func DirectoryVisit(ds *DirectoryState, setID int, wayID int) {
	set := &ds.Sets[setID]

	for i, w := range set.LRUOrder {
		if w == wayID {
			set.LRUOrder = append(set.LRUOrder[:i], set.LRUOrder[i+1:]...)

			break
		}
	}

	set.LRUOrder = append(set.LRUOrder, wayID)
}

// DirectoryReset initializes a DirectoryState with numSets*numWays empty
// blocks and default LRU ordering.
func DirectoryReset(ds *DirectoryState, numSets, numWays, blockSize int) {
	ds.Sets = make([]SetState, numSets)

	for i := 0; i < numSets; i++ {
		ds.Sets[i].Blocks = make([]BlockState, numWays)
		ds.Sets[i].LRUOrder = make([]int, numWays)

		for j := 0; j < numWays; j++ {
			ds.Sets[i].Blocks[j] = BlockState{
				SetID:        i,
				WayID:        j,
				CacheAddress: uint64(i*numWays+j) * uint64(blockSize),
			}
			ds.Sets[i].LRUOrder[j] = j
		}
	}
}
