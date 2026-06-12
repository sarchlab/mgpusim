package cache

import "github.com/sarchlab/akita/v5/mem/vm"

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
	setID := int(addr / uint64(blockSize) % uint64(numSets))
	set := &ds.Sets[setID]

	for wayID, block := range set.Blocks {
		if block.IsValid && block.Tag == addr && vm.PID(block.PID) == pid {
			return setID, wayID, true
		}
	}

	return setID, -1, false
}

// DirectoryFindVictim returns (setID, wayID) of the LRU victim for addr.
// The victim is the block at LRUOrder[0] (least recently used).
func DirectoryFindVictim(
	ds *DirectoryState,
	numSets int,
	blockSize int,
	addr uint64,
) (int, int) {
	setID := int(addr / uint64(blockSize) % uint64(numSets))
	set := &ds.Sets[setID]

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
