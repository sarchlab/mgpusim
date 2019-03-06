package driver

import (
	"log"

	"github.com/rs/xid"
	"gitlab.com/akita/mem/vm"
)

// GPUPtr is the type that represent a pointer pointing into the GPU memory
type GPUPtr uint64

// LocalPtr is a type that represent a pointer to a region in the LDS memory
type LocalPtr uint32

// A MemoryChunk is a piece of allocated or free memory.
type MemoryChunk struct {
	Ptr      GPUPtr
	ByteSize uint64
	Occupied bool
}

func (d *Driver) registerStorage(
	loAddr GPUPtr,
	byteSize uint64,
) {
	d.memoryMasks = append(d.memoryMasks, make([]*MemoryChunk, 0))
	d.allocatedPages = append(d.allocatedPages, make([]*vm.Page, 0))

	d.initialAddresses = append(d.initialAddresses,
		d.totalStorageByteSize)
	d.storageSizes = append(d.storageSizes, byteSize)
}

// AllocateMemory allocates a chunk of memory of size byteSize in storage.
// It returns the pointer pointing to the newly allocated memory in the GPU
// memory space.
func (d *Driver) AllocateMemory(
	c *Context,
	byteSize uint64,
) GPUPtr {
	if byteSize >= 8 {
		return d.AllocateMemoryWithAlignment(c, byteSize, 8)
	}
	return d.AllocateMemoryWithAlignment(c, byteSize, byteSize)
}

// AllocateMemoryWithAlignment allocates a chunk of memory of size byteSize.
// The return address must be a multiple of alignment.
func (d *Driver) AllocateMemoryWithAlignment(
	c *Context,
	byteSize, alignment uint64,
) GPUPtr {
	if byteSize >= 4096 {
		return d.allocateLarge(c, byteSize)
	}

	ptr, ok := d.tryAllocateWithExistingChunks(
		c.CurrentGPUID, byteSize, alignment)
	if ok {
		return ptr
	}

	d.allocatePage(c, 1<<d.PageSizeAsPowerOf2)

	ptr, ok = d.tryAllocateWithExistingChunks(
		c.CurrentGPUID, byteSize, alignment)
	if ok {
		return ptr
	}

	log.Panic("never")
	return 0
}

// Remap keeps the virtual address unchanged and moves the physical address to
// another GPU
func (d *Driver) Remap(addr, size uint64, gpuID int) {
	// ptr := addr
	// sizeLeft := size
	// for ptr < addr+size {
	// 	_, page := d.MMU.Translate(d.currentPID, ptr)
	// 	d.remapPage(page, ptr, sizeLeft, gpuID)
	// 	sizeLeft -= page.VAddr + page.PageSize - ptr
	// 	ptr = page.VAddr + page.PageSize
	// }
}

func (d *Driver) remapPage(page *vm.Page, addr, size uint64, gpuID int) {
	//ptr := addr
	//d.MMU.RemovePage(d.currentPID, page.VAddr)
	//if ptr > page.VAddr {
	//	page1 := &vm.Page{
	//		PID:      d.currentPID,
	//		VAddr:    page.VAddr,
	//		PAddr:    page.PAddr,
	//		PageSize: addr - page.VAddr,
	//		Valid:    true,
	//	}
	//	d.MMU.CreatePage(page1)
	//}
	//
	//sizeLeft := page.PageSize - (addr - page.VAddr)
	//sizeForNewPage := sizeLeft
	//if sizeForNewPage > size {
	//	sizeForNewPage = size
	//}
	//d.allocatePageWithGivenVAddr(gpuID, addr, sizeForNewPage)
	////d.mmu.CreatePage(page2)
	//
	//ptr += sizeForNewPage
	//sizeLeft -= sizeForNewPage
	//if sizeLeft > 0 {
	//	page3 := &vm.Page{
	//		PID:      d.currentPID,
	//		VAddr:    ptr,
	//		PAddr:    page.PAddr + (ptr - page.VAddr),
	//		PageSize: sizeLeft,
	//		Valid:    true,
	//	}
	//	d.MMU.CreatePage(page3)
	//}
}

func (d *Driver) allocateLarge(c *Context, byteSize uint64) GPUPtr {
	gpuID := c.CurrentGPUID
	pageSize := uint64(1 << d.PageSizeAsPowerOf2)
	numPages := (byteSize-1)/pageSize + 1

	pageID := d.initialAddresses[gpuID]
	for pageID < d.initialAddresses[gpuID]+d.storageSizes[gpuID] {
		free := true
		for i := uint64(0); i < numPages; i++ {
			if d.isPageAllocated(gpuID, pageID+i*pageSize) {
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
		page := &vm.Page{
			PID:      c.PID,
			VAddr:    pageID + i*pageSize + 0x100000000,
			PAddr:    pageID + i*pageSize,
			PageSize: pageSize,
			Valid:    true,
		}
		d.allocatedPages[gpuID] = append(d.allocatedPages[gpuID], page)
		d.MMU.CreatePage(page)
	}

	return GPUPtr(pageID + 0x100000000)
}

func (d *Driver) allocatePage(c *Context, size uint64) *vm.Page {
	gpuID := c.CurrentGPUID
	pageID := d.initialAddresses[gpuID]
	for pageID < d.initialAddresses[gpuID]+d.storageSizes[gpuID] {
		if d.isPageAllocated(gpuID, pageID) {
			pageID += 1 << d.PageSizeAsPowerOf2
		} else {
			break
		}
	}

	page := &vm.Page{
		PID:      c.PID,
		VAddr:    pageID + 0x100000000,
		PAddr:    pageID,
		PageSize: size,
		Valid:    true,
	}

	virtualAddr := pageID + 0x100000000

	d.MMU.CreatePage(page)
	d.allocatedPages[gpuID] = append(d.allocatedPages[gpuID], page)

	chunk := new(MemoryChunk)
	chunk.Ptr = GPUPtr(virtualAddr)
	chunk.ByteSize = 1 << d.PageSizeAsPowerOf2
	chunk.Occupied = false
	d.memoryMasks[gpuID] = append(d.memoryMasks[gpuID], chunk)

	return page
}

func (d *Driver) allocatePageWithGivenVAddr(c *Context, vAddr, size uint64) *vm.Page {
	gpuID := c.CurrentGPUID
	pageID := d.initialAddresses[gpuID]
	for pageID < d.initialAddresses[gpuID]+d.storageSizes[gpuID] {
		if d.isPageAllocated(gpuID, pageID) {
			pageID += 1 << d.PageSizeAsPowerOf2
		} else {
			break
		}
	}

	page := &vm.Page{
		PID:      c.PID,
		VAddr:    vAddr,
		PAddr:    pageID,
		PageSize: size,
		Valid:    true,
	}
	//virtualAddr := pageID + 0x100000000

	d.MMU.CreatePage(page)
	d.allocatedPages[gpuID] = append(d.allocatedPages[gpuID], page)

	//chunk := new(MemoryChunk)
	//chunk.Ptr = GPUPtr(virtualAddr)
	//chunk.ByteSize = 1 << d.PageSizeAsPowerOf2
	//chunk.Occupied = false
	//d.memoryMasks[gpuID] = append(d.memoryMasks[gpuID], chunk)

	return page
}

func (d *Driver) isPageAllocated(gpuID int, pAddr uint64) bool {
	for _, p := range d.allocatedPages[gpuID] {
		if p.PAddr == pAddr {
			return true
		}
	}
	return false
}

func (d *Driver) tryAllocateWithExistingChunks(
	gpuID int,
	byteSize, alignment uint64,
) (ptr GPUPtr, ok bool) {
	chunks := d.memoryMasks[gpuID]
	for i, chunk := range chunks {
		if chunk.Occupied {
			continue
		}

		nextAlignment := ((uint64(chunk.Ptr)-1)/alignment + 1) * alignment
		if nextAlignment <= uint64(chunk.Ptr)+chunk.ByteSize &&
			nextAlignment+byteSize <= uint64(chunk.Ptr)+chunk.ByteSize {

			ptr = GPUPtr(nextAlignment)
			ok = true

			d.splitChunk(gpuID, i, ptr, byteSize)

			return
		}
	}

	return 0, false
}

func (d *Driver) splitChunk(gpuID, chunkIndex int, ptr GPUPtr, byteSize uint64) {
	chunks := d.memoryMasks[gpuID]
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
	d.memoryMasks[gpuID] = newChunks
}

// FreeMemory frees the memory pointed by ptr. The pointer must be allocated
// with the function AllocateMemory earlier. Error will be returned if the ptr
// provided is invalid.
func (d *Driver) FreeMemory(ptr GPUPtr) error {
	//mask := d.memoryMasks[d.usingGPU]
	//chunks := mask.Chunks
	//for i := 0; i < len(chunks); i++ {
	//	if chunks[i].Ptr == ptr {
	//		chunks[i].Occupied = false
	//
	//		if i != 0 && i != len(chunks)-1 && chunks[i-1].Occupied == false && chunks[i+1].Occupied == false {
	//			chunks[i-1].ByteSize += chunks[i].ByteSize + chunks[i+1].ByteSize
	//			mask.Chunks = append(chunks[:i], chunks[i+2:]...)
	//			return nil
	//		}
	//
	//		if i != 0 && chunks[i-1].Occupied == false {
	//			chunks[i-1].ByteSize += chunks[i].ByteSize
	//			mask.Chunks = append(chunks[:i], chunks[i+1:]...)
	//			return nil
	//		}
	//
	//		if i != len(chunks)-1 && chunks[i+1].Occupied == false {
	//			chunks[i].ByteSize += chunks[i+1].ByteSize
	//			mask.Chunks = append(chunks[:i+1], chunks[i+2:]...)
	//			return nil
	//		}
	//		return nil
	//	}
	//}
	//
	//log.Fatalf("Invalid pointer")
	return nil
}

// EnqueueMemCopyH2D registers a MemCopyH2DCommand in the queue.
func (d *Driver) EnqueueMemCopyH2D(
	queue *CommandQueue,
	dst GPUPtr,
	src interface{},
) {
	cmd := &MemCopyH2DCommand{
		ID:  xid.New().String(),
		Dst: dst,
		Src: src,
	}

	queue.Commands = append(queue.Commands, cmd)
	d.enqueueSignal <- true
}

// EnqueueMemCopyD2H registers a MemCopyD2HCommand in the queue.
func (d *Driver) EnqueueMemCopyD2H(
	queue *CommandQueue,
	dst interface{},
	src GPUPtr,
) {
	cmd := &MemCopyD2HCommand{
		ID:  xid.New().String(),
		Dst: dst,
		Src: src,
	}
	queue.Commands = append(queue.Commands, cmd)
	d.enqueueSignal <- true
}

// MemCopyH2D copies a memory from the host to a GPU device.
func (d *Driver) MemCopyH2D(ctx *Context, dst GPUPtr, src interface{}) {
	queue := d.CreateCommandQueue(ctx)
	d.EnqueueMemCopyH2D(queue, dst, src)
	d.DrainCommandQueue(queue)
}

// MemCopyD2H copies a memory from a GPU device to the host
func (d *Driver) MemCopyD2H(ctx *Context, dst interface{}, src GPUPtr) {
	queue := d.CreateCommandQueue(ctx)
	d.EnqueueMemCopyD2H(queue, dst, src)
	d.DrainCommandQueue(queue)
}
