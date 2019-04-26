package driver

import (
	"log"
	"sync"

	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/util/ca"
)

// A memoryAllocator can allocate memory on the CPU and GPUs
type memoryAllocator interface {
	RegisterStorage(byteSize uint64)
	GetDeviceIDByPAddr(pAddr uint64) int
	Allocate(ctx *Context, byteSize uint64) GPUPtr
	AllocateWithAlignment(ctx *Context, byteSize, alignment uint64) GPUPtr
	Free(ctx *Context, ptr GPUPtr)
	Remap(ctx *Context, pageVAddr, byteSize uint64, deviceID int)
}

// A memoryChunk is a piece of allocated or free memory.
type memoryChunk struct {
	ptr      uint64
	byteSize uint64
	occupied bool
}

// A memoryAllocatorImpl provides the default implementation for
// memoryAllocator
type memoryAllocatorImpl struct {
	sync.Mutex

	mmu vm.MMU

	pageSizeAsPowerOf2 uint64

	allocatedPages       [][]vm.Page
	initialAddresses     []uint64
	storageSizes         []uint64
	memoryMasks          [][]*memoryChunk
	totalStorageByteSize uint64
}

func newMemoryAllocatorImpl(mmu vm.MMU) *memoryAllocatorImpl {
	a := &memoryAllocatorImpl{
		mmu: mmu,
	}
	return a
}

func (a *memoryAllocatorImpl) RegisterStorage(
	byteSize uint64,
) {
	a.memoryMasks = append(a.memoryMasks, make([]*memoryChunk, 0))
	a.allocatedPages = append(a.allocatedPages, make([]vm.Page, 0))

	a.initialAddresses = append(a.initialAddresses,
		a.totalStorageByteSize)
	a.storageSizes = append(a.storageSizes, byteSize)

	a.totalStorageByteSize += byteSize
}

func (a *memoryAllocatorImpl) GetDeviceIDByPAddr(pAddr uint64) int {
	for i := 0; i < len(a.initialAddresses); i++ {
		if pAddr >= a.initialAddresses[i] &&
			pAddr < a.initialAddresses[i]+a.storageSizes[i] {
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
		return a.AllocateWithAlignment(ctx, byteSize, 8)
	}
	return a.AllocateWithAlignment(ctx, byteSize, byteSize)
}

func (a *memoryAllocatorImpl) AllocateWithAlignment(
	ctx *Context,
	byteSize, alignment uint64,
) GPUPtr {
	a.Lock()
	defer a.Unlock()

	if byteSize >= 4096 {
		return a.allocateLarge(ctx, byteSize)
	}

	ptr, ok := a.tryAllocateWithExistingChunks(
		ctx.currentGPUID, byteSize, alignment)
	if ok {
		return GPUPtr(ptr)
	}

	a.allocatePage(ctx)

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
	byteSize uint64,
) GPUPtr {
	gpuID := ctx.currentGPUID
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	numPages := (byteSize-1)/pageSize + 1

	pageID := a.initialAddresses[gpuID]
	for pageID < a.initialAddresses[gpuID]+a.storageSizes[gpuID] {
		free := true
		for i := uint64(0); i < numPages; i++ {
			if a.isPageAllocated(gpuID, pageID+i*pageSize) {
				free = false
				break
			}
		}

		if !free {
			pageID += pageSize
		} else {
			break
		}
	}

	ptr := ctx.prevPageVAddr + pageSize
	for i := uint64(0); i < numPages; i++ {
		ctx.prevPageVAddr += pageSize
		page := vm.Page{
			PID:      ctx.pid,
			VAddr:    ctx.prevPageVAddr,
			PAddr:    pageID + i*pageSize,
			PageSize: pageSize,
			Valid:    true,
		}
		a.allocatedPages[gpuID] = append(a.allocatedPages[gpuID], page)
		a.mmu.CreatePage(&page)
	}

	return GPUPtr(ptr)
}

func (a *memoryAllocatorImpl) tryAllocateWithExistingChunks(
	deviceID int,
	byteSize, alignment uint64,
) (ptr uint64, ok bool) {
	chunks := a.memoryMasks[deviceID]
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
	chunks := a.memoryMasks[deviceID]
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
	a.memoryMasks[deviceID] = newChunks
}

func (a *memoryAllocatorImpl) allocatePage(ctx *Context) vm.Page {
	deviceID := ctx.currentGPUID
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)

	pAddr := a.initialAddresses[deviceID]
	for pAddr < a.initialAddresses[deviceID]+a.storageSizes[deviceID] {
		if a.isPageAllocated(deviceID, pAddr) {
			pAddr += pageSize
		} else {
			break
		}
	}

	vAddr := ctx.prevPageVAddr + pageSize
	ctx.prevPageVAddr = vAddr
	page := vm.Page{
		PID:      ctx.pid,
		VAddr:    vAddr,
		PAddr:    pAddr,
		PageSize: pageSize,
		Valid:    true,
	}

	a.mmu.CreatePage(&page)
	a.allocatedPages[deviceID] = append(a.allocatedPages[deviceID], page)

	chunk := new(memoryChunk)
	chunk.ptr = vAddr
	chunk.byteSize = pageSize
	chunk.occupied = false
	a.memoryMasks[deviceID] = append(a.memoryMasks[deviceID], chunk)

	return page
}

func (a *memoryAllocatorImpl) isPageAllocated(deviceID int, pAddr uint64) bool {
	for _, p := range a.allocatedPages[deviceID] {
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
		a.removePage(ctx.pid, addr)
		a.allocatePageWithGivenVAddr(ctx, deviceID, addr)
		a.migrateChunks(addr, deviceID)
		addr += pageSize
	}
}

func (a *memoryAllocatorImpl) migrateChunks(pageVAddr uint64, deviceID int) {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	for i, memoryMask := range a.memoryMasks {
		if i == deviceID {
			continue
		}

		newMemoryMask := []*memoryChunk{}
		for _, chunk := range memoryMask {
			addr := chunk.ptr

			if addr >= pageVAddr && addr < pageVAddr+pageSize {
				a.memoryMasks[deviceID] =
					append(a.memoryMasks[deviceID], chunk)
				continue
			}

			newMemoryMask = append(newMemoryMask, chunk)
		}

		a.memoryMasks[i] = newMemoryMask
	}
}

func (a *memoryAllocatorImpl) removePage(pid ca.PID, addr uint64) {
	for i, pages := range a.allocatedPages {
		newPages := []vm.Page{}
		for _, page := range pages {
			if page.PID != pid || page.VAddr != addr {
				newPages = append(newPages, page)
				continue
			}
		}
		a.allocatedPages[i] = newPages
	}
	a.mmu.RemovePage(pid, addr)
}

func (a *memoryAllocatorImpl) removePageChunks(vAddr uint64) {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	for i, chunks := range a.memoryMasks {
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
		a.memoryMasks[i] = chunks
	}
}

func (a *memoryAllocatorImpl) allocatePageWithGivenVAddr(
	ctx *Context,
	deviceID int,
	vAddr uint64,
) vm.Page {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
	pAddr := a.initialAddresses[deviceID]
	for pAddr < a.initialAddresses[deviceID]+a.storageSizes[deviceID] {
		if a.isPageAllocated(deviceID, pAddr) {
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
	}

	a.mmu.CreatePage(&page)
	a.allocatedPages[deviceID] = append(a.allocatedPages[deviceID], page)

	return page
}

func (a *memoryAllocatorImpl) Free(ctx *Context, ptr GPUPtr) {
	panic("not implemented")
}
