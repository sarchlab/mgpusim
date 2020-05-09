package internal

import (
	"sync"

	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/util/ca"
)

// NewBuddyAllocator creates a new buddy memory allocator.
func NewBuddyAllocator (
	pageTable vm.PageTable,
	log2PageSize uint64,
) MemoryAllocator {
		a := &buddyAllocatorImpl{
			pageTable:            pageTable,
			totalStorageByteSize: 4096, // Starting with a page to avoid 0 address.
			log2PageSize:         log2PageSize,
			processMemoryStates:  make(map[ca.PID]*processMemoryState),
			vAddrToPageMapping:   make(map[uint64]vm.Page),
			devices:              make(map[int]*Device),
		}
	return a
}

// A buddyAllocatorImpl provides buddy algorithm implementation for
// memoryAllocator
type buddyAllocatorImpl struct {
	sync.Mutex
	pageTable            vm.PageTable
	log2PageSize         uint64
	vAddrToPageMapping   map[uint64]vm.Page
	processMemoryStates  map[ca.PID]*processMemoryState
	devices              map[int]*Device
	totalStorageByteSize uint64
}

func (b *buddyAllocatorImpl) RegisterDevice(device *Device) {
	b.Lock()
	defer b.Unlock()

	state := device.memState
	state.setInitialAddress(b.totalStorageByteSize)

	b.totalStorageByteSize += state.getStorageSize()

	b.devices[device.ID] = device
}

func (b *buddyAllocatorImpl) GetDeviceIDByPAddr(pAddr uint64) int {
	return -1
}

func (b *buddyAllocatorImpl) Allocate(
	pid ca.PID,
	byteSize uint64,
	deviceID int,
) uint64 {
		return 0
}

func (b *buddyAllocatorImpl) AllocateUnified(
	pid ca.PID,
	byteSize uint64,
) uint64 {
		return 0
}

func (b *buddyAllocatorImpl) Remap(
	pid ca.PID,
	pageVAddr, byteSize uint64,
	deviceID int,
) {

}

func (b *buddyAllocatorImpl) RemovePage(vAddr uint64) {

}

func (b *buddyAllocatorImpl) AllocatePageWithGivenVAddr(
	pid ca.PID,
	deviceID int,
	vAddr uint64,
	isUnified bool,
) vm.Page {
		return vm.Page{}
}

func (b *buddyAllocatorImpl) Free(ptr uint64) {

}