package internal

import (
	"container/list"
)

func newDeviceBuddyMemoryState(log2pagesize uint64) deviceMemoryState {
	return &deviceBuddyMemoryState{
		log2PageSize: log2pagesize,
	}
}

// A deviceBuddyMemoryState implements deviceMemoryState as a buddy allocator
type deviceBuddyMemoryState struct {
	initFlag bool

	log2PageSize   uint64
	initialAddress uint64
	storageSize    uint64
	freeList       []list.List
	bfBlockSplit   *bitField
	bfMergeList    *bitField
	blockTracking  map[uint64]*blockTracker
}

func (bms *deviceBuddyMemoryState) setInitialAddress(addr uint64) {
	bms.initialAddress = addr
	bms.blockTracking = make(map[uint64]*blockTracker)

	if len(bms.freeList) != 0 {
		bms.freeList[0].PushBack(addr)
		bms.initFlag = false
		return
	}
	bms.initFlag = true
}

func (bms *deviceBuddyMemoryState) getInitialAddress() uint64 {
	return bms.initialAddress
}

func (bms *deviceBuddyMemoryState) setStorageSize(size uint64) {
	bms.storageSize = size
	var order int
	for order = int(bms.log2PageSize); (1 << order) < size; order++ {}
	order -= int(bms.log2PageSize)
	bms.freeList = make([]list.List, order+1)
	for _, l := range bms.freeList {
		l.Init()
	}

	bms.bfBlockSplit = newBitField(1 << order)
	bms.bfMergeList = newBitField(1 << order)

	if bms.initFlag {
		bms.freeList[0].PushBack(bms.initialAddress)
	}
}

func (bms *deviceBuddyMemoryState) getStorageSize() uint64 {
	return bms.storageSize
}

func (bms *deviceBuddyMemoryState) addSinglePAddr(addr uint64) {
	if bt, ok := bms.blockTracking[addr]; ok {
		delete(bms.blockTracking, addr)
		if bt.removePage() {
			bms.freeBlock(bt.initialAddr)
		}
	}
}

func (bms *deviceBuddyMemoryState) popNextAvailablePAddrs() uint64 {
	addrs := bms.allocateMultiplePages(1)
	return  addrs[0]
}

func (bms *deviceBuddyMemoryState) noAvailablePAddrs() bool {
	for _, fList := range bms.freeList {
		if fList.Len() != 0 {
			return false
		}
	}
	return true
}

func (bms *deviceBuddyMemoryState) allocateMultiplePages(
	numPages int,
) (pAddrs []uint64) {
	freeListLen := len(bms.freeList) - 1

	var order int
	for order = 12; (1 << order) < (numPages * 4096); order++ {}
	level := freeListLen - (order - 12)

	i := level

	for {
		if i < 0 {
			panic("not enough memory available")
		}
		if bms.freeList[i].Len() != 0 {
			break
		}
		i--
	}
	e := bms.freeList[i].Front()
	block := bms.freeList[i].Remove(e).(uint64)

	if i == level && i > 0 {
		bms.updateMergeListBitField(bms.indexOfBlock(block, i-1))
	}

	for i < level {
		bms.updateSplitBlockBitField(bms.indexOfBlock(block, i))
		bms.updateMergeListBitField(bms.indexOfBlock(block, i))
		i++
		buddy := bms.buddyOf(block, i)
		bms.freeList[i].PushBack(buddy)
	}

	bTracker := &blockTracker{
		initialAddr: block,
		numOfPages:  numPages,
	}

	for j := 0; j < numPages; j++ {
		pAddrs = append(pAddrs, block)
		bms.blockTracking[block] = bTracker
		block += 4096
	}

	return pAddrs
}

func (bms *deviceBuddyMemoryState) buddyOf(addr uint64, level int) (buddy uint64) {
	if bms.indexInLevelOf(addr, level) % 2 == 0 {
		buddy = addr + bms.sizeOfLevel(level)
	} else {
		buddy = addr - bms.sizeOfLevel(level)
	}
	return
}

func (bms *deviceBuddyMemoryState) indexOfBlock(ptr uint64, level int) uint64 {
	return (1 << level) + bms.indexInLevelOf(ptr,level) - 1
}

func (bms *deviceBuddyMemoryState) indexInLevelOf(ptr uint64, level int) uint64 {
	return (ptr - bms.initialAddress) / bms.sizeOfLevel(level)
}

func (bms *deviceBuddyMemoryState) sizeOfLevel(level int) uint64 {
	return bms.storageSize / (1 << level)
}

func (bms *deviceBuddyMemoryState) updateSplitBlockBitField(index uint64) {
	bms.bfBlockSplit.updateBit(index)
}

func (bms *deviceBuddyMemoryState) updateMergeListBitField(index uint64) {
	bms.bfMergeList.updateBit(index)
}

func (bms *deviceBuddyMemoryState) freeBlock(addr uint64) {
	level := bms.levelOfBlock(addr)
	for level > 0 {
		bms.updateMergeListBitField(bms.indexOfBlock(addr, level-1))
		if !bms.blockOrBuddyIsAllocated(addr, level) {
			bms.updateSplitBlockBitField(bms.indexOfBlock(addr, level-1))
			buddy := bms.buddyOf(addr,level)
			removeByValue(&bms.freeList[level], buddy)
			if buddy < addr {
				addr = buddy
			}
			level--
		} else {
			bms.freeList[level].PushBack(addr)
			return
		}
	}
	bms.freeList[level].PushBack(addr)
}

func removeByValue(l *list.List, fAddr uint64) bool {
	if l.Len() == 0 {
		return false
	}

	for e := l.Front(); e != nil; e = e.Next() {
		if e.Value.(uint64) == fAddr {
			l.Remove(e)
			return true
		}
	}
	return false
}

func (bms *deviceBuddyMemoryState) levelOfBlock(addr uint64) int {
	n := len(bms.freeList) - 1
	for n > 0 {
		if bms.blockHasBeenSplit(addr, n-1) {
			return n
		}
		n--
	}
	return 0
}

func (bms *deviceBuddyMemoryState) blockHasBeenSplit(ptr uint64, level int) bool {
	index := bms.indexOfBlock(ptr, level)
	return bms.bfBlockSplit.checkBit(index)
}

func (bms *deviceBuddyMemoryState) blockOrBuddyIsAllocated(ptr uint64, level int) bool {
	index := bms.indexOfBlock(ptr, level - 1)
	return bms.bfMergeList.checkBit(index)
}