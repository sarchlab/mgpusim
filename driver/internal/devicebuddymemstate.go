package internal

func newDeviceBuddyMemoryState() deviceMemoryState {
	return &deviceBuddyMemoryState{}
}

//buddy allocation implementation of deviceMemoryState
type deviceBuddyMemoryState struct {
	initialAddress  uint64
	storageSize     uint64
	freeList        []*freeListElement
	//bfBlockSplit    uint64
	//bfFreeBlocks    uint64
}

func (bms *deviceBuddyMemoryState) setInitialAddress(addr uint64) {
	bms.initialAddress = addr
	pushBack(&bms.freeList[0], addr)
}

func (bms *deviceBuddyMemoryState) getInitialAddress() uint64 {
	return bms.initialAddress
}

func (bms *deviceBuddyMemoryState) setStorageSize(size uint64) {
	bms.storageSize = size
	var order int
	for order = 12; (1 << order) < size; order++ {}
	order -= 12
	bms.freeList = make([]*freeListElement, order+1)
}

func (bms *deviceBuddyMemoryState) getStorageSize() uint64 {
	return bms.storageSize
}

func (bms *deviceBuddyMemoryState) addSinglePAddr(addr uint64) {

}

func (bms *deviceBuddyMemoryState) popNextAvailablePAddrs() uint64 {
	addrs := bms.allocateMultiplePages(1)
	return  addrs[0]
}

func (bms *deviceBuddyMemoryState) noAvailablePAddrs() bool {
	for _, fList := range bms.freeList {
		if fList != nil {
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
		if bms.freeList[i] != nil {
			break
		}
		i--
	}

	block := popFront(&bms.freeList[i])

	for i < level {
		i++
		buddy := bms.buddyOf(block, i)
		pushBack(&bms.freeList[i], buddy)
	}

	for j := 0; j < numPages; j++ {
		pAddrs = append(pAddrs, block)
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