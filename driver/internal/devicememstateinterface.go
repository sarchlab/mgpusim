package internal

type DeviceMemoryState interface {
	setInitialAddress(addr uint64)
	getInitialAddress() uint64
	setStorageSize(size uint64)
	getStorageSize() uint64
	addSinglePAddr(addr uint64)
	popNextAvailablePAddrs() uint64
	noAvailablePAddrs() bool
	allocateMultiplePages(numPages int) []uint64
}

// NewDeviceMemoryState creates a new device memory state based on allocator type.
func NewDeviceMemoryState(log2pagesize uint64) DeviceMemoryState {
	switch MemoryAllocatorType {
	case AllocatorTypeDefault:
		return newDeviceRegularMemoryState(log2pagesize)
	case AllocatorTypeBuddy:
		return newDeviceBuddyMemoryState(log2pagesize)
	default:
		panic("Invalid memory allocator type")
	}
}

func newDeviceRegularMemoryState(log2pagesize uint64) DeviceMemoryState {
	return &deviceMemoryStateImpl{
		log2PageSize: log2pagesize,
	}
}

//original implementation of DeviceMemoryState holding free addresses in array
type deviceMemoryStateImpl struct {
	log2PageSize    uint64
	initialAddress  uint64
	storageSize     uint64
	availablePAddrs []uint64
}

func (dms *deviceMemoryStateImpl) setInitialAddress(addr uint64) {
	dms.initialAddress = addr

	pageSize := uint64(1 << dms.log2PageSize)
	endAddr := dms.initialAddress + dms.storageSize
	for addr := dms.initialAddress; addr < endAddr; addr += pageSize {
		dms.addSinglePAddr(addr)
	}
}

func (dms *deviceMemoryStateImpl) getInitialAddress() uint64 {
		return dms.initialAddress
}

func (dms *deviceMemoryStateImpl) setStorageSize(size uint64) {
	dms.storageSize = size
}

func (dms *deviceMemoryStateImpl) getStorageSize() uint64 {
	return dms.storageSize
}

func (dms *deviceMemoryStateImpl) addSinglePAddr(addr uint64) {
	dms.availablePAddrs = append(dms.availablePAddrs, addr)
}

func (dms *deviceMemoryStateImpl) popNextAvailablePAddrs() uint64  {
	nextPAddr := dms.availablePAddrs[0]
	dms.availablePAddrs = dms.availablePAddrs[1:]
	return  nextPAddr
}

func (dms *deviceMemoryStateImpl) noAvailablePAddrs() bool {
	return len(dms.availablePAddrs) == 0
}

func (dms *deviceMemoryStateImpl) allocateMultiplePages(
	numPages int,
) (pAddrs []uint64) {
	for i := 0; i < numPages; i++ {
		pAddr := dms.popNextAvailablePAddrs()
		pAddrs = append(pAddrs, pAddr)
	}
	return pAddrs
}