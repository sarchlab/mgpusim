package internal

func newDeviceBuddyMemoryState() deviceMemoryState {
	return &deviceBuddyMemoryState{}
}

//buddy allocation implementation of deviceMemoryState
type deviceBuddyMemoryState struct {
	initialAddress  uint64
	storageSize     uint64
	freeList        []*freeListElement
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

func (bms *deviceBuddyMemoryState) popNextAvailablePAddrs() uint64  {

	return  0
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
	iOrder := order

	for {
		if i < 0 {
			panic("not enough memory available")
		}
		if bms.freeList[i] != nil {
			break
		}
		i--
		iOrder++
	}

	block := popFront(&bms.freeList[i])

	for i < level {
		i++
		iOrder--
		buddy := bms.buddyOf(block, iOrder)
		pushBack(&bms.freeList[i], buddy)
	}

	for j := 0; j < numPages; j++ {
		pAddrs = append(pAddrs, block)
		block += 4096
	}

	return pAddrs
}

func (bms *deviceBuddyMemoryState) buddyOf(addr uint64, order int) uint64 {
	offset := addr - bms.initialAddress
	buddy := (offset ^ (1 << order)) + bms.initialAddress
	return buddy
}