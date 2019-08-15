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

<<<<<<< HEAD
=======
type deviceMemoryState struct {
	allocatedPages []vm.Page
	initialAddress uint64
	storageSize    uint64
	nextPAddr      uint64
	memoryChunks   []*memoryChunk
}

func (s *deviceMemoryState) updateNextPAddr(pageSize uint64) {
	s.nextPAddr += pageSize
	if s.nextPAddr > s.initialAddress+s.storageSize {
		panic("memory is full")
	}
}

>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
// A memoryAllocatorImpl provides the default implementation for
// memoryAllocator
type memoryAllocatorImpl struct {
	sync.Mutex

	mmu mmu.MMU

	pageSizeAsPowerOf2 uint64

<<<<<<< HEAD
	allocatedPages       [][]vm.Page
	initialAddresses     []uint64
	storageSizes         []uint64
	memoryMasks          [][]*memoryChunk
=======
	deviceMemoryStates   []deviceMemoryState
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
<<<<<<< HEAD
	a.memoryMasks = append(a.memoryMasks, make([]*memoryChunk, 0))
	a.allocatedPages = append(a.allocatedPages, make([]vm.Page, 0))

	a.initialAddresses = append(a.initialAddresses,
		a.totalStorageByteSize)
	a.storageSizes = append(a.storageSizes, byteSize)
=======
	state := deviceMemoryState{}
	state.storageSize = byteSize
	state.initialAddress = a.totalStorageByteSize
	state.nextPAddr = a.totalStorageByteSize
	a.deviceMemoryStates = append(a.deviceMemoryStates, state)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c

	a.totalStorageByteSize += byteSize
}

func (a *memoryAllocatorImpl) GetDeviceIDByPAddr(pAddr uint64) int {
<<<<<<< HEAD
	for i := 0; i < len(a.initialAddresses); i++ {
		if pAddr >= a.initialAddresses[i] &&
			pAddr < a.initialAddresses[i]+a.storageSizes[i] {
=======
	for i := 0; i < len(a.deviceMemoryStates); i++ {
		if pAddr >= a.deviceMemoryStates[i].initialAddress &&
			pAddr < a.deviceMemoryStates[i].initialAddress+
				a.deviceMemoryStates[i].storageSize {
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
		ptr := a.allocateLarge(ctx, byteSize)
		return ptr
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

<<<<<<< HEAD
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
=======
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
		}
		ctx.prevPageVAddr = vAddr
		a.deviceMemoryStates[gpuID].updateNextPAddr(pageSize)
		a.deviceMemoryStates[gpuID].allocatedPages = append(
			a.deviceMemoryStates[gpuID].allocatedPages, page)
		a.mmu.CreatePage(&page)
	}

	return GPUPtr(initVAddr)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
}

func (a *memoryAllocatorImpl) tryAllocateWithExistingChunks(
	deviceID int,
	byteSize, alignment uint64,
) (ptr uint64, ok bool) {
<<<<<<< HEAD
	chunks := a.memoryMasks[deviceID]
=======
	chunks := a.deviceMemoryStates[deviceID].memoryChunks
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
<<<<<<< HEAD
	chunks := a.memoryMasks[deviceID]
=======
	chunks := a.deviceMemoryStates[deviceID].memoryChunks
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
<<<<<<< HEAD
	a.memoryMasks[deviceID] = newChunks
=======
	a.deviceMemoryStates[deviceID].memoryChunks = newChunks
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
}

func (a *memoryAllocatorImpl) allocatePage(ctx *Context) vm.Page {
	deviceID := ctx.currentGPUID
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)

<<<<<<< HEAD
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
=======
	pAddr := a.deviceMemoryStates[deviceID].nextPAddr
	vAddr := ctx.prevPageVAddr + pageSize
	ctx.prevPageVAddr = vAddr
	a.deviceMemoryStates[deviceID].updateNextPAddr(pageSize)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
	page := vm.Page{
		PID:      ctx.pid,
		VAddr:    vAddr,
		PAddr:    pAddr,
		PageSize: pageSize,
		Valid:    true,
	}

	a.mmu.CreatePage(&page)
<<<<<<< HEAD
	a.allocatedPages[deviceID] = append(a.allocatedPages[deviceID], page)
=======
	a.deviceMemoryStates[deviceID].allocatedPages = append(
		a.deviceMemoryStates[deviceID].allocatedPages, page)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c

	chunk := new(memoryChunk)
	chunk.ptr = vAddr
	chunk.byteSize = pageSize
	chunk.occupied = false
<<<<<<< HEAD
	a.memoryMasks[deviceID] = append(a.memoryMasks[deviceID], chunk)
=======
	a.deviceMemoryStates[deviceID].memoryChunks = append(
		a.deviceMemoryStates[deviceID].memoryChunks, chunk)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c

	return page
}

<<<<<<< HEAD
func (a *memoryAllocatorImpl) isPageAllocated(deviceID int, pAddr uint64) bool {
	for _, p := range a.allocatedPages[deviceID] {
		if p.PAddr == pAddr {
			return true
		}
	}
	return false
}

=======
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
<<<<<<< HEAD
	for i, memoryMask := range a.memoryMasks {
=======
	for i := 0; i < len(a.deviceMemoryStates); i++ {
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
		if i == deviceID {
			continue
		}

<<<<<<< HEAD
		newMemoryMask := []*memoryChunk{}
		for _, chunk := range memoryMask {
			addr := chunk.ptr

			if addr >= pageVAddr && addr < pageVAddr+pageSize {
				a.memoryMasks[deviceID] =
					append(a.memoryMasks[deviceID], chunk)
=======
		state := &a.deviceMemoryStates[i]

		newMemoryMask := []*memoryChunk{}
		for _, chunk := range state.memoryChunks {
			addr := chunk.ptr

			if addr >= pageVAddr && addr < pageVAddr+pageSize {
				a.deviceMemoryStates[deviceID].memoryChunks =
					append(a.deviceMemoryStates[deviceID].memoryChunks, chunk)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
				continue
			}

			newMemoryMask = append(newMemoryMask, chunk)
		}

<<<<<<< HEAD
		a.memoryMasks[i] = newMemoryMask
=======
		state.memoryChunks = newMemoryMask
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
	}
}

func (a *memoryAllocatorImpl) removePage(pid ca.PID, addr uint64) {
<<<<<<< HEAD
	for i, pages := range a.allocatedPages {
=======
	for i := 0; i < len(a.deviceMemoryStates); i++ {
		state := &a.deviceMemoryStates[i]
		pages := state.allocatedPages
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
		newPages := []vm.Page{}
		for _, page := range pages {
			if page.PID != pid || page.VAddr != addr {
				newPages = append(newPages, page)
				continue
			}
		}
<<<<<<< HEAD
		a.allocatedPages[i] = newPages
=======
		state.allocatedPages = newPages
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
	}
	a.mmu.RemovePage(pid, addr)
}

func (a *memoryAllocatorImpl) removePageChunks(vAddr uint64) {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
<<<<<<< HEAD
	for i, chunks := range a.memoryMasks {
=======
	for i := 0; i < len(a.deviceMemoryStates); i++ {
		state := &a.deviceMemoryStates[i]
		chunks := state.memoryChunks
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
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
<<<<<<< HEAD
		a.memoryMasks[i] = chunks
=======
		state.memoryChunks = chunks
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c
	}
}

func (a *memoryAllocatorImpl) allocatePageWithGivenVAddr(
	ctx *Context,
	deviceID int,
	vAddr uint64,
) vm.Page {
	pageSize := uint64(1 << a.pageSizeAsPowerOf2)
<<<<<<< HEAD
	pAddr := a.initialAddresses[deviceID]
	for pAddr < a.initialAddresses[deviceID]+a.storageSizes[deviceID] {
		if a.isPageAllocated(deviceID, pAddr) {
			pAddr += pageSize
		} else {
			break
		}
	}
=======
	pAddr := a.deviceMemoryStates[deviceID].nextPAddr
	a.deviceMemoryStates[deviceID].updateNextPAddr(pageSize)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c

	page := vm.Page{
		PID:      ctx.pid,
		VAddr:    vAddr,
		PAddr:    pAddr,
		PageSize: pageSize,
		Valid:    true,
	}

<<<<<<< HEAD
	a.mmu.CreatePage(&page)
	a.allocatedPages[deviceID] = append(a.allocatedPages[deviceID], page)
=======
	a.deviceMemoryStates[deviceID].allocatedPages = append(
		a.deviceMemoryStates[deviceID].allocatedPages, page)
	a.mmu.CreatePage(&page)
>>>>>>> 12541da0d25788542564ac324fb8ad31b05e7d5c

	return page
}

func (a *memoryAllocatorImpl) Free(ctx *Context, ptr GPUPtr) {
	panic("not implemented")
}
