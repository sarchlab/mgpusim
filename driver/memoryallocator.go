package driver

import (
	"log"
	"sync"

	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/util/ca"
)

// A memoryAllocator can allocate memory on the CPU and GPUs
type memoryAllocator interface {
	RegisterStorage(byteSize uint64)
	GetDeviceIDByPAddr(pAddr uint64) int
	Allocate(ctx *Context, byteSize uint64) GPUPtr
	AllocateUnified(ctx *Context, byteSize uint64) GPUPtr
	AllocateWithAlignment(ctx *Context, byteSize, alignment uint64, unified bool) GPUPtr
	Free(ctx *Context, ptr GPUPtr)
	Remap(ctx *Context, pageVAddr, byteSize uint64, deviceID int)
	AllocatePageWithGivenVAddr(ctx *Context, deviceID int, vAddr uint64, unified bool) vm.Page
	RemovePage(pid ca.PID, addr uint64, unified bool)
	MigrateChunks(pageVAddr uint64, deviceID int)
}

// A memoryChunk is a piece of allocated or free memory.
type memoryChunk struct {
	ptr      uint64
	byteSize uint64
	occupied bool
}

type deviceMemoryState struct {
	allocatedPages        []vm.Page
	allocatedUnifiedPages []vm.Page
	initialAddress        uint64
	storageSize           uint64
	nextPAddr             uint64
	memoryChunks          []*memoryChunk
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

	mmu mmu.MMU

	pageSizeAsPowerOf2 uint64

	deviceMemoryStates   []deviceMemoryState
	totalStorageByteSize uint64
}

func newMemoryAllocatorImpl(mmu mmu.MMU) *memoryAllocatorImpl {
	a := &memoryAllocatorImpl{
		mmu: mmu,
	}
	return a
}

func (a *memoryAllocatorImpl) RegisterStorage(
	byteSize uint64,
) {
	state := deviceMemoryState{}
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
	ctx *Context,
	byteSize uint64,
) GPUPtr {
	if byteSize >= 8 {
		return a.AllocateWithAlignment(ctx, byteSize, 8, false)
	}
	return a.AllocateWithAlignment(ctx, byteSize, byteSize, false)
}

func (a *memoryAllocatorImpl) AllocateUnified(
	ctx *Context,
	byteSize uint64,
) GPUPtr {

	if byteSize >= 8 {
		return a.AllocateWithAlignment(ctx, byteSize, 8, true)
	}
	return a.AllocateWithAlignment(ctx, byteSize, byteSize, true)
}

func (a *memoryAllocatorImpl) AllocateWithAlignment(
	ctx *Context,
	byteSize, alignment uint64, unified bool,
) GPUPtr {
	a.Lock()
	defer a.Unlock()

	if byteSize >= 4096 {
		return a.allocateLarge(ctx, byteSize, unified)
	}

	ptr, ok := a.tryAllocateWithExistingChunks(
		ctx.currentGPUID, byteSize, alignment)
	if ok {
		return GPUPtr(ptr)
	}

	a.allocatePage(ctx, unified)

	ptr, ok = a.tryAllocateWithExistingChunks(
		ctx.currentGPUID, byteSize, alignment)
	if ok {
		return GPUPtr(ptr)
	}

	log.Panic("never")
	return 0
}

func (a *memoryAllocatorImpl) allocateLarge(
	ctx *Context,
	byteSize uint64, unified bool,
) GPUPtr {

	gpuID := ctx.currentGPUID
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	numPages := (byteSize-1)/pageSize + 1
	initVAddr := ctx.prevPageVAddr + pageSize

	for i := uint64(0); i < numPages; i++ {
		pAddr := a.deviceMemoryStates[gpuID].nextPAddr
		vAddr := ctx.prevPageVAddr + pageSize

		page := vm.Page{
			PID:      ctx.pid,
			VAddr:    vAddr,
			PAddr:    pAddr,
			PageSize: pageSize,
			Valid:    true,
			Unified:  unified,
			GPUID:    uint64(ctx.currentGPUID),
		}

		ctx.prevPageVAddr = vAddr
		a.deviceMemoryStates[gpuID].updateNextPAddr(pageSize)
		a.deviceMemoryStates[gpuID].allocatedPages = append(
			a.deviceMemoryStates[gpuID].allocatedPages, page)
		a.mmu.CreatePage(&page)

	}
	return GPUPtr(initVAddr)

}

func (a *memoryAllocatorImpl) tryAllocateWithExistingChunks(
	deviceID int,
	byteSize, alignment uint64,
) (ptr uint64, ok bool) {
	chunks := a.deviceMemoryStates[deviceID].memoryChunks
	for i, chunk := range chunks {
		if chunk.occupied {
			continue
		}

		nextAlignment := ((chunk.ptr-1)/alignment + 1) * alignment
		if nextAlignment <= chunk.ptr+chunk.byteSize &&
			nextAlignment+byteSize <= chunk.ptr+chunk.byteSize {

			ptr = nextAlignment
			ok = true

			a.splitChunk(deviceID, i, ptr, byteSize)

			return
		}
	}

	return 0, false

}

func (a *memoryAllocatorImpl) splitChunk(
	deviceID int,
	chunkIndex int,
	ptr uint64,
	byteSize uint64,
) {
	chunks := a.deviceMemoryStates[deviceID].memoryChunks
	chunk := chunks[chunkIndex]
	newChunks := chunks[:chunkIndex]

	if ptr != chunk.ptr {
		newChunk1 := new(memoryChunk)
		newChunk1.byteSize = ptr - chunk.ptr
		newChunk1.ptr = ptr
		newChunk1.occupied = false
		newChunks = append(newChunks, newChunk1)
	}

	newChunk2 := new(memoryChunk)
	newChunk2.byteSize = byteSize
	newChunk2.ptr = ptr
	newChunk2.occupied = true
	newChunks = append(newChunks, newChunk2)

	if ptr+byteSize < chunk.ptr+chunk.byteSize {
		newChunk3 := new(memoryChunk)
		newChunk3.ptr = ptr + byteSize
		newChunk3.byteSize = chunk.ptr + chunk.byteSize - (ptr + byteSize)
		newChunk3.occupied = false
		newChunks = append(newChunks, newChunk3)
	}

	newChunks = append(newChunks, chunks[chunkIndex+1:]...)
	a.deviceMemoryStates[deviceID].memoryChunks = newChunks

}

func (a *memoryAllocatorImpl) allocatePage(ctx *Context, unified bool) vm.Page {
	deviceID := ctx.currentGPUID
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)

	pAddr := a.deviceMemoryStates[deviceID].nextPAddr
	vAddr := ctx.prevPageVAddr + pageSize
	ctx.prevPageVAddr = vAddr
	a.deviceMemoryStates[deviceID].updateNextPAddr(pageSize)
	page := vm.Page{
		PID:      ctx.pid,
		VAddr:    vAddr,
		PAddr:    pAddr,
		PageSize: pageSize,
		Valid:    true,
		Unified:  unified,
		GPUID:    uint64(ctx.currentGPUID),
	}

	a.mmu.CreatePage(&page)

	if unified {
		a.deviceMemoryStates[deviceID].allocatedUnifiedPages = append(a.deviceMemoryStates[deviceID].allocatedUnifiedPages, page)
	} else {
		a.deviceMemoryStates[deviceID].allocatedPages = append(
			a.deviceMemoryStates[deviceID].allocatedPages, page)
	}

	chunk := new(memoryChunk)
	chunk.ptr = vAddr
	chunk.byteSize = pageSize
	chunk.occupied = false

	a.deviceMemoryStates[deviceID].memoryChunks = append(
		a.deviceMemoryStates[deviceID].memoryChunks, chunk)

	return page
}

func (a *memoryAllocatorImpl) isPageAllocated(deviceID int, pAddr uint64, unified bool) bool {


	for _, p := range a.deviceMemoryStates[deviceID].allocatedUnifiedPages {
		if p.PAddr == pAddr {
			return true
		}
	}

	for _, p := range a.deviceMemoryStates[deviceID].allocatedPages {
		if p.PAddr == pAddr {
			return true
		}
	}

	return false

}

func (a *memoryAllocatorImpl) Remap(
	ctx *Context,
	pageVAddr, byteSize uint64,
	deviceID int,
) {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	addr := pageVAddr
	for addr < pageVAddr+byteSize {
		a.RemovePage(ctx.pid, addr, false)
		a.AllocatePageWithGivenVAddr(ctx, deviceID, addr, false)
		a.MigrateChunks(addr, deviceID)
		addr += pageSize
	}
}

func (a *memoryAllocatorImpl) MigrateChunks(pageVAddr uint64, deviceID int) {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	for i := 0; i < len(a.deviceMemoryStates); i++ {
		if i == deviceID {
			continue
		}

		state := &a.deviceMemoryStates[i]

		newMemoryMask := []*memoryChunk{}

		for _, chunk := range state.memoryChunks {
			addr := chunk.ptr

			if addr >= pageVAddr && addr < pageVAddr+pageSize {

				a.deviceMemoryStates[deviceID].memoryChunks =
					append(a.deviceMemoryStates[deviceID].memoryChunks, chunk)
				continue
			}

			newMemoryMask = append(newMemoryMask, chunk)
		}

		state.memoryChunks = newMemoryMask

	}
}

func (a *memoryAllocatorImpl) RemovePage(pid ca.PID, addr uint64, unified bool) {
	if unified == true {

		for i := 0; i < len(a.deviceMemoryStates); i++ {
			state := &a.deviceMemoryStates[i]
			pages := state.allocatedUnifiedPages
			newPages := []vm.Page{}
			for _, page := range pages {
				if page.PID != pid || page.VAddr != addr {
					newPages = append(newPages, page)
					continue
				}
			}

		}

	} else {
		for i := 0; i < len(a.deviceMemoryStates); i++ {
			state := &a.deviceMemoryStates[i]
			pages := state.allocatedPages
			newPages := []vm.Page{}
			for _, page := range pages {
				if page.PID != pid || page.VAddr != addr {
					newPages = append(newPages, page)
					continue
				}
			}

		}
	}

	a.mmu.RemovePage(pid, addr)
}

func (a *memoryAllocatorImpl) removePageChunks(vAddr uint64) {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	for i := 0; i < len(a.deviceMemoryStates); i++ {
		state := &a.deviceMemoryStates[i]
		chunks := state.memoryChunks
		newChunks := []*memoryChunk{}
		for _, chunk := range chunks {
			addr := chunk.ptr
			if addr < vAddr || addr >= vAddr+pageSize {
				newChunks = append(newChunks, chunk)
			} else {
				if chunk.occupied {
					log.Panic("Memory still in use")
				}
			}
		}
		state.memoryChunks = chunks
	}

}



func (a *memoryAllocatorImpl) AllocatePageWithGivenVAddr(
	ctx *Context,
	deviceID int,
	vAddr uint64,
	unified bool,
) vm.Page {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	pAddr := a.deviceMemoryStates[deviceID].initialAddress
	for pAddr < a.deviceMemoryStates[deviceID].initialAddress+a.deviceMemoryStates[deviceID].storageSize {
		if a.isPageAllocated(deviceID, pAddr, unified) {
			pAddr += pageSize
		} else {
			break
		}
	}


	page := vm.Page{
		PID:      ctx.pid,
		VAddr:    vAddr,
		PAddr:    pAddr,
		PageSize: pageSize,
		Valid:    true,
		GPUID:    uint64(deviceID),
		Unified:  unified,
	}

	a.mmu.CreatePage(&page)

	if unified {
		a.deviceMemoryStates[deviceID].allocatedUnifiedPages = append(a.deviceMemoryStates[deviceID].allocatedUnifiedPages, page)
	} else {
		a.deviceMemoryStates[deviceID].allocatedPages = append(a.deviceMemoryStates[deviceID].allocatedPages, page)
	}

	return page
}



func (a *memoryAllocatorImpl) Free(ctx *Context, ptr GPUPtr) {
	panic("not implemented")
}
