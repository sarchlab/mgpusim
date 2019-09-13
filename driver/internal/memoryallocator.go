package internal

import (
	"log"
	"sync"

	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/util/ca"
)

// A MemoryAllocator can allocate memory on the CPU and GPUs
type MemoryAllocator interface {
	RegisterStorage(byteSize uint64)
	GetDeviceIDByPAddr(pAddr uint64) int
	Allocate(pid ca.PID, byteSize uint64, deviceID int) uint64
	AllocateUnified(pid ca.PID, byteSize uint64) uint64
	Free(vAddr uint64)
	Remap(pid ca.PID, pageVAddr, byteSize uint64, deviceID int)
	AllocatePageWithGivenVAddr(
		pid ca.PID,
		deviceID int,
		vAddr uint64,
		unified bool,
	) vm.Page
}

type processMemoryState struct {
	pid       ca.PID
	nextVAddr uint64
}

type deviceMemoryState struct {
	allocatedPages        []vm.Page
	allocatedUnifiedPages []vm.Page
	initialAddress        uint64
	storageSize           uint64
	nextPAddr             uint64
}

func (s *deviceMemoryState) updateNextPAddr(pageSize uint64) {
	s.nextPAddr += pageSize
	if s.nextPAddr > s.initialAddress+s.storageSize {
		panic("memory is full")
	}
}

// A memoryAllocatorImpl provides the default implementation for
// memoryAllocator
type memoryAllocatorImpl struct {
	sync.Mutex
	mmu                  mmu.MMU
	log2PageSize         uint64
	vAddrToPageMapping   map[uint64]vm.Page
	processMemoryStates  map[ca.PID]*processMemoryState
	deviceMemoryStates   []*deviceMemoryState
	totalStorageByteSize uint64
}

func newMemoryAllocatorImpl(mmu mmu.MMU) *memoryAllocatorImpl {
	a := &memoryAllocatorImpl{
		mmu:                 mmu,
		processMemoryStates: make(map[ca.PID]*processMemoryState),
		vAddrToPageMapping:  make(map[uint64]vm.Page),
	}
	return a
}

func (a *memoryAllocatorImpl) RegisterStorage(
	byteSize uint64,
) {
	state := &deviceMemoryState{}
	state.storageSize = byteSize
	state.initialAddress = a.totalStorageByteSize
	state.nextPAddr = a.totalStorageByteSize
	a.deviceMemoryStates = append(a.deviceMemoryStates, state)

	a.totalStorageByteSize += byteSize
}

func (a *memoryAllocatorImpl) GetDeviceIDByPAddr(pAddr uint64) int {
	for i := 0; i < len(a.deviceMemoryStates); i++ {
		if pAddr >= a.deviceMemoryStates[i].initialAddress &&
			pAddr < a.deviceMemoryStates[i].initialAddress+
				a.deviceMemoryStates[i].storageSize {
			return i
		}
	}

	log.Panic("device not found")
	return 0
}

func (a *memoryAllocatorImpl) Allocate(
	pid ca.PID,
	byteSize uint64,
	deviceID int,
) uint64 {
	pageSize := uint64(1 << a.log2PageSize)
	numPages := (byteSize-1)/pageSize + 1
	return a.allocatePages(int(numPages), pid, deviceID, false)
}

func (a *memoryAllocatorImpl) AllocateUnified(
	pid ca.PID,
	byteSize uint64,
) uint64 {
	pageSize := uint64(1 << a.log2PageSize)
	numPages := (byteSize-1)/pageSize + 1
	return a.allocatePages(int(numPages), pid, 0, true)
}

func (a *memoryAllocatorImpl) allocatePages(
	numPages int,
	pid ca.PID,
	deviceID int,
	unified bool,
) (firstPageVAddr uint64) {
	pState, found := a.processMemoryStates[pid]
	if !found {
		a.processMemoryStates[pid] = &processMemoryState{
			pid:       pid,
			nextVAddr: 4096,
		}
		pState = a.processMemoryStates[pid]
	}
	dState := a.deviceMemoryStates[deviceID]

	pageSize := uint64(1 << a.log2PageSize)
	nextVAddr := pState.nextVAddr
	nextPAddr := dState.nextPAddr

	for i := 0; i < numPages; i++ {
		pAddr := nextPAddr + uint64(i)*pageSize
		vAddr := nextVAddr + uint64(i)*pageSize

		page := vm.Page{
			PID:      pid,
			VAddr:    vAddr,
			PAddr:    pAddr,
			PageSize: pageSize,
			Valid:    true,
			Unified:  unified,
			GPUID:    uint64(deviceID),
		}
		a.vAddrToPageMapping[page.VAddr] = page
		a.mmu.CreatePage(&page)
	}

	dState.nextPAddr += pageSize * uint64(numPages)
	pState.nextVAddr += pageSize * uint64(numPages)

	return nextVAddr
}

func (a *memoryAllocatorImpl) Remap(
	pid ca.PID,
	pageVAddr, byteSize uint64,
	deviceID int,
) {
	pageSize := uint64(1 << a.log2PageSize)
	addr := pageVAddr
	for addr < pageVAddr+byteSize {
		a.removePage(addr)
		a.AllocatePageWithGivenVAddr(pid, deviceID, addr, false)
		addr += pageSize
	}
}

func (a *memoryAllocatorImpl) removePage(vAddr uint64) {
	page, ok := a.vAddrToPageMapping[vAddr]

	if !ok {
		panic("page not found")
	}

	isUnified := page.Unified
	deviceID := a.GetDeviceIDByPAddr(page.PAddr)
	dState := a.deviceMemoryStates[deviceID]

	if isUnified {
		newPages := []vm.Page{}
		for _, p := range dState.allocatedUnifiedPages {
			if p != page {
				newPages = append(newPages, p)
			}
		}
		dState.allocatedUnifiedPages = newPages
	} else {
		newPages := []vm.Page{}
		for _, p := range dState.allocatedPages {
			if p != page {
				newPages = append(newPages, p)
			}
		}
		dState.allocatedPages = newPages
	}

	a.mmu.RemovePage(page.PID, page.VAddr)
}

func (a *memoryAllocatorImpl) AllocatePageWithGivenVAddr(
	pid ca.PID,
	deviceID int,
	vAddr uint64,
	isUnified bool,
) vm.Page {
	pageSize := uint64(1 << a.log2PageSize)
	dState := a.deviceMemoryStates[deviceID]

	page := vm.Page{
		PID:      pid,
		VAddr:    vAddr,
		PAddr:    dState.nextPAddr,
		PageSize: pageSize,
		Valid:    true,
		GPUID:    uint64(deviceID),
		Unified:  isUnified,
	}
	a.vAddrToPageMapping[page.VAddr] = page

	a.mmu.CreatePage(&page)

	if isUnified {
		dState.allocatedUnifiedPages = append(dState.allocatedUnifiedPages,
			page)
	} else {
		dState.allocatedPages = append(dState.allocatedPages, page)
	}

	return page
}

func (a *memoryAllocatorImpl) Free(ptr uint64) {
	a.removePage(ptr)
}
