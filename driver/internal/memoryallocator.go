// Package internal provides support for the driver implementation.
package internal

import (
	"sync"

	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/util/ca"
)

// A MemoryAllocator can allocate memory on the CPU and GPUs
type MemoryAllocator interface {
	RegisterDevice(device *Device)
	GetDeviceIDByPAddr(pAddr uint64) int
	Allocate(pid ca.PID, byteSize uint64, deviceID int) uint64
	AllocateUnified(pid ca.PID, byteSize uint64) uint64
	Free(vAddr uint64)
	Remap(pid ca.PID, pageVAddr, byteSize uint64, deviceID int)
	RemovePage(vAddr uint64)
	AllocatePageWithGivenVAddr(
		pid ca.PID,
		deviceID int,
		vAddr uint64,
		unified bool,
	) vm.Page
}

// NewMemoryAllocator creates a new memory allocator.
func NewMemoryAllocator(
	pageTable vm.PageTable,
	log2PageSize uint64,
) MemoryAllocator {
	a := &memoryAllocatorImpl{
		pageTable:            pageTable,
		totalStorageByteSize: 4096, // Starting with a page to avoid 0 address.
		log2PageSize:         log2PageSize,
		processMemoryStates:  make(map[ca.PID]*processMemoryState),
		vAddrToPageMapping:   make(map[uint64]vm.Page),
		devices:              make(map[int]*Device),
	}
	return a
}

type processMemoryState struct {
	pid       ca.PID
	nextVAddr uint64
}

// A memoryAllocatorImpl provides the default implementation for
// memoryAllocator
type memoryAllocatorImpl struct {
	sync.Mutex
	pageTable            vm.PageTable
	log2PageSize         uint64
	vAddrToPageMapping   map[uint64]vm.Page
	processMemoryStates  map[ca.PID]*processMemoryState
	devices              map[int]*Device
	totalStorageByteSize uint64
}

func (a *memoryAllocatorImpl) RegisterDevice(device *Device) {
	a.Lock()
	defer a.Unlock()

	if device.memState == nil {
		switch MemoryAllocatorType {
		case allocatorTypeDefault:
			device.memState = newDeviceRegularMemoryState()
		case allocatorTypeBuddy:
			device.memState = newDeviceBuddyMemoryState()
		}
	}

	state := device.memState
	state.setInitialAddress(a.totalStorageByteSize)

	pageSize := uint64(1 << a.log2PageSize)
	endAddr := state.getInitialAddress() + state.getStorageSize()
	for addr := state.getInitialAddress(); addr < endAddr; addr += pageSize {
		state.addSinglePAddr(addr)
	}

	a.totalStorageByteSize += state.getStorageSize()

	a.devices[device.ID] = device
}

func (a *memoryAllocatorImpl) GetDeviceIDByPAddr(pAddr uint64) int {
	a.Lock()
	defer a.Unlock()

	return a.deviceIDByPAddr(pAddr)
}

func (a *memoryAllocatorImpl) deviceIDByPAddr(pAddr uint64) int {
	for id, dev := range a.devices {
		state := dev.memState
		if isPAddrOnDevice(pAddr, state) {
			return id
		}
	}

	panic("device not found")
}

func isPAddrOnDevice(
	pAddr uint64,
	state deviceMemoryState,
) bool {
	return pAddr >= state.getInitialAddress() &&
		pAddr < state.getInitialAddress()+state.getStorageSize()
}

func (a *memoryAllocatorImpl) Allocate(
	pid ca.PID,
	byteSize uint64,
	deviceID int,
) uint64 {
	a.Lock()
	defer a.Unlock()

	pageSize := uint64(1 << a.log2PageSize)
	numPages := (byteSize-1)/pageSize + 1
	return a.allocatePages(int(numPages), pid, deviceID, false)
}

func (a *memoryAllocatorImpl) AllocateUnified(
	pid ca.PID,
	byteSize uint64,
) uint64 {
	a.Lock()
	defer a.Unlock()

	pageSize := uint64(1 << a.log2PageSize)
	numPages := (byteSize-1)/pageSize + 1
	return a.allocatePages(int(numPages), pid, 1, true)
}

func (a *memoryAllocatorImpl) allocatePages(
	numPages int,
	pid ca.PID,
	deviceID int,
	unified bool,
) (firstPageVAddr uint64) {
	// log.Printf("num pages %d \n", numPages)
	pState, found := a.processMemoryStates[pid]
	if !found {
		a.processMemoryStates[pid] = &processMemoryState{
			pid:       pid,
			nextVAddr: uint64(1 << a.log2PageSize),
		}
		pState = a.processMemoryStates[pid]
	}
	device := a.devices[deviceID]

	pageSize := uint64(1 << a.log2PageSize)
	nextVAddr := pState.nextVAddr

	for i := 0; i < numPages; i++ {
		pAddr := device.allocatePage()
		vAddr := nextVAddr + uint64(i)*pageSize

		page := vm.Page{
			PID:      pid,
			VAddr:    vAddr,
			PAddr:    pAddr,
			PageSize: pageSize,
			Valid:    true,
			Unified:  unified,
			GPUID:    uint64(a.deviceIDByPAddr(pAddr)),
		}

		a.pageTable.Insert(page)
		a.vAddrToPageMapping[page.VAddr] = page
	}

	pState.nextVAddr += pageSize * uint64(numPages)

	return nextVAddr
}

func (a *memoryAllocatorImpl) Remap(
	pid ca.PID,
	pageVAddr, byteSize uint64,
	deviceID int,
) {
	a.Lock()
	defer a.Unlock()

	pageSize := uint64(1 << a.log2PageSize)
	addr := pageVAddr
	for addr < pageVAddr+byteSize {
		// a.removePage(addr)
		a.allocatePageWithGivenVAddr(pid, deviceID, addr, false)
		addr += pageSize
	}
}

func (a *memoryAllocatorImpl) RemovePage(vAddr uint64) {
	a.Lock()
	defer a.Unlock()

	a.removePage(vAddr)
}

func (a *memoryAllocatorImpl) removePage(vAddr uint64) {
	page, ok := a.vAddrToPageMapping[vAddr]

	if !ok {
		panic("page not found")
	}

	deviceID := a.deviceIDByPAddr(page.PAddr)
	dState := a.devices[deviceID].memState
	dState.addSinglePAddr(page.PAddr)

	a.pageTable.Remove(page.PID, page.VAddr)
}

func (a *memoryAllocatorImpl) AllocatePageWithGivenVAddr(
	pid ca.PID,
	deviceID int,
	vAddr uint64,
	isUnified bool,
) vm.Page {
	a.Lock()
	defer a.Unlock()

	return a.allocatePageWithGivenVAddr(pid, deviceID, vAddr, isUnified)
}

func (a *memoryAllocatorImpl) allocatePageWithGivenVAddr(
	pid ca.PID,
	deviceID int,
	vAddr uint64,
	isUnified bool,
) vm.Page {
	pageSize := uint64(1 << a.log2PageSize)

	device := a.devices[deviceID]
	pAddr := device.allocatePage()

	page := vm.Page{
		PID:      pid,
		VAddr:    vAddr,
		PAddr:    pAddr,
		PageSize: pageSize,
		Valid:    true,
		GPUID:    uint64(deviceID),
		Unified:  isUnified,
	}
	a.vAddrToPageMapping[page.VAddr] = page
	a.pageTable.Update(page)

	return page
}

func (a *memoryAllocatorImpl) Free(ptr uint64) {
	a.Lock()
	defer a.Unlock()

	a.removePage(ptr)
}
