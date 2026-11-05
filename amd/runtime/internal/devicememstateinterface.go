package internal

// A DeviceMemoryState allocates the physical pages of a device. Physical
// addresses only increase: a freed page is retired and never allocated
// again, so no cache can hold a line of a newly allocated page.
type DeviceMemoryState interface {
	setInitialAddress(addr uint64)
	getInitialAddress() uint64
	setStorageSize(size uint64)
	getStorageSize() uint64
	retirePAddr(addr uint64)
	popNextAvailablePAddrs() uint64
	noAvailablePAddrs() bool
	allocateMultiplePages(numPages int) []uint64
}

// NewDeviceMemoryState creates the memory state of a device.
func NewDeviceMemoryState(log2pagesize uint64) DeviceMemoryState {
	return &deviceMemoryStateImpl{log2PageSize: log2pagesize}
}

// deviceMemoryStateImpl allocates pages in increasing address order.
type deviceMemoryStateImpl struct {
	log2PageSize   uint64
	initialAddress uint64
	storageSize    uint64
	nextPAddr      uint64
}

func (dms *deviceMemoryStateImpl) setInitialAddress(addr uint64) {
	dms.initialAddress = addr
	dms.nextPAddr = addr
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

// retirePAddr retires a freed page. It is never allocated again.
func (dms *deviceMemoryStateImpl) retirePAddr(uint64) {}

func (dms *deviceMemoryStateImpl) popNextAvailablePAddrs() uint64 {
	pAddr := dms.nextPAddr
	dms.nextPAddr += 1 << dms.log2PageSize

	return pAddr
}

func (dms *deviceMemoryStateImpl) noAvailablePAddrs() bool {
	return dms.nextPAddr+(1<<dms.log2PageSize) >
		dms.initialAddress+dms.storageSize
}

func (dms *deviceMemoryStateImpl) allocateMultiplePages(
	numPages int,
) (pAddrs []uint64) {
	for range numPages {
		if dms.noAvailablePAddrs() {
			panic("out of memory")
		}

		pAddrs = append(pAddrs, dms.popNextAvailablePAddrs())
	}

	return pAddrs
}
