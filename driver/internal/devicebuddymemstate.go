package internal

func newDeviceBuddyMemoryState(size uint64) deviceMemoryState {
	bms := &deviceBuddyMemoryState{
		storageSize: size,
	}
	return bms
}

//buddy allocation implementation of deviceMemoryState
type deviceBuddyMemoryState struct {
	initialAddress  uint64
	storageSize     uint64
}

func (bms *deviceBuddyMemoryState) setInitialAddress(addr uint64) {
	bms.initialAddress = addr
}

func (bms *deviceBuddyMemoryState) getInitialAddress() uint64 {
	return bms.initialAddress
}

func (bms *deviceBuddyMemoryState) setStorageSize(size uint64) {
	bms.storageSize = size
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
	return false
}