package driver

import (
	"log"
	"sync"

	"gitlab.com/akita/mem/vm"
)

//go:generate mockgen -destination mock_driver/mock_memoryallocator.go -source $GOFILE

// A memoryAllocator can allocate memory on the CPU and GPUs
type memoryAllocator interface {
	RegisterStorage(initAddr, byteSize uint64)
	Allocate(ctx *Context, byteSize uint64) GPUPtr
	AllocateWithAlignment(ctx *Context, byteSize, alignment uint64) GPUPtr
	Free(ctx *Context, ptr GPUPtr)
	Remap(ctx *Context, pageVAddr, byteSize uint64, deviceID int)
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
	memoryMasks          [][]*MemoryChunk
	totalStorageByteSize uint64
}

func newMemoryAllocatorImpl(mmu vm.MMU) *memoryAllocatorImpl {
	a := &memoryAllocatorImpl{
		mmu: mmu,
	}
	return a
}

func (a *memoryAllocatorImpl) RegisterStorage(
	initAddr uint64, byteSize uint64,
) {
	a.memoryMasks = append(a.memoryMasks, make([]*MemoryChunk, 0))
	a.allocatedPages = append(a.allocatedPages, make([]vm.Page, 0))

	a.initialAddresses = append(a.initialAddresses,
		a.totalStorageByteSize)
	a.storageSizes = append(a.storageSizes, byteSize)

	a.totalStorageByteSize += byteSize
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
		return ptr
	}

	a.allocatePage(ctx)

	ptr, ok = a.tryAllocateWithExistingChunks(
		ctx.currentGPUID, byteSize, alignment)
	if ok {
		return ptr
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

	for i := uint64(0); i < numPages; i++ {
		page := vm.Page{
			PID:      ctx.pid,
			VAddr:    pageID + i*pageSize + 0x100000000,
			PAddr:    pageID + i*pageSize,
			PageSize: pageSize,
			Valid:    true,
		}
		a.allocatedPages[gpuID] = append(a.allocatedPages[gpuID], page)
		a.mmu.CreatePage(&page)
	}

	return GPUPtr(pageID + 0x100000000)
}

func (a *memoryAllocatorImpl) tryAllocateWithExistingChunks(
	deviceID int,
	byteSize, alignment uint64,
) (ptr GPUPtr, ok bool) {
	chunks := a.memoryMasks[deviceID]
	for i, chunk := range chunks {
		if chunk.Occupied {
			continue
		}

		nextAlignment := ((uint64(chunk.Ptr)-1)/alignment + 1) * alignment
		if nextAlignment <= uint64(chunk.Ptr)+chunk.ByteSize &&
			nextAlignment+byteSize <= uint64(chunk.Ptr)+chunk.ByteSize {

			ptr = GPUPtr(nextAlignment)
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
	ptr GPUPtr,
	byteSize uint64,
) {
	chunks := a.memoryMasks[deviceID]
	chunk := chunks[chunkIndex]
	newChunks := chunks[:chunkIndex]

	if ptr != chunk.Ptr {
		newChunk1 := new(MemoryChunk)
		newChunk1.ByteSize = uint64(ptr - chunk.Ptr)
		newChunk1.Ptr = ptr
		newChunk1.Occupied = false
		newChunks = append(newChunks, newChunk1)
	}

	newChunk2 := new(MemoryChunk)
	newChunk2.ByteSize = byteSize
	newChunk2.Ptr = ptr
	newChunk2.Occupied = true
	newChunks = append(newChunks, newChunk2)

	if uint64(ptr)+byteSize < uint64(chunk.Ptr)+chunk.ByteSize {
		newChunk3 := new(MemoryChunk)
		newChunk3.Ptr = GPUPtr(uint64(ptr) + byteSize)
		newChunk3.ByteSize = uint64(chunk.Ptr) + chunk.ByteSize -
			(uint64(ptr) + byteSize)
		newChunk3.Occupied = false
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

	page := vm.Page{
		PID:      ctx.pid,
		VAddr:    pAddr + 0x100000000,
		PAddr:    pAddr,
		PageSize: pageSize,
		Valid:    true,
	}

	virtualAddr := pAddr + 0x100000000

	a.mmu.CreatePage(&page)
	a.allocatedPages[deviceID] = append(a.allocatedPages[deviceID], page)

	chunk := new(MemoryChunk)
	chunk.Ptr = GPUPtr(virtualAddr)
	chunk.ByteSize = pageSize
	chunk.Occupied = false
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

		newMemoryMask := []*MemoryChunk{}
		for _, chunk := range memoryMask {
			addr := uint64(chunk.Ptr)

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

func (a *memoryAllocatorImpl) removePage(pid vm.PID, addr uint64) {
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
		newChunks := []*MemoryChunk{}
		for _, chunk := range chunks {
			addr := uint64(chunk.Ptr)
			if addr < vAddr || addr >= vAddr+pageSize {
				newChunks = append(newChunks, chunk)
			} else {
				if chunk.Occupied {
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
