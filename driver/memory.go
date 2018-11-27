package driver

import (
	"log"

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
	byteSize uint64,
) GPUPtr {
	if byteSize >= 8 {
		return d.AllocateMemoryWithAlignment(byteSize, 8)
	}
	return d.AllocateMemoryWithAlignment(byteSize, byteSize)
}

// AllocateMemoryWithAlignment allocates a chunk of memory of size byteSize.
// The return address must be a multiple of alignment.
func (d *Driver) AllocateMemoryWithAlignment(
	byteSize uint64,
	alignment uint64,
) GPUPtr {
	if byteSize >= 4096 {
		return d.allocateLarge(byteSize)
	}

	ptr, ok := d.tryAllocateWithExistingChunks(byteSize, alignment)
	if ok {
		return ptr
	}

	d.allocatePage()

	ptr, ok = d.tryAllocateWithExistingChunks(byteSize, alignment)
	if ok {
		return ptr
	}

	log.Panic("Something wrong happened!")
	return 0
}

func (d *Driver) allocateLarge(byteSize uint64) GPUPtr {
	pageSize := uint64(1 << d.PageSizeAsPowerOf2)
	numPages := (byteSize-1)/pageSize + 1

	pageID := d.initialAddresses[d.usingGPU]
	for pageID < d.initialAddresses[d.usingGPU]+d.storageSizes[d.usingGPU] {
		free := true
		for i := uint64(0); i < numPages; i++ {
			if d.isPageAllocated(pageID + i*pageSize) {
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
		page := vm.NewPage()
		page.PhysicalFrameNumber = pageID>>d.PageSizeAsPowerOf2 + i
		d.allocatedPages[d.usingGPU] = append(d.allocatedPages[d.usingGPU], page)

		virtualAddr := pageID + 0x100000000

		d.mmu.CreatePage(d.currentPID,
			pageID>>d.PageSizeAsPowerOf2+i,
			virtualAddr+i*pageSize,
			1<<d.PageSizeAsPowerOf2)
	}

	return GPUPtr(pageID + 0x100000000)
}

func (d *Driver) allocatePage() {
	pageID := d.initialAddresses[d.usingGPU]
	for pageID < d.initialAddresses[d.usingGPU]+d.storageSizes[d.usingGPU] {
		if d.isPageAllocated(pageID) {
			pageID += 1 << d.PageSizeAsPowerOf2
		} else {
			break
		}
	}

	virtualAddr := pageID + 0x100000000
	d.mmu.CreatePage(d.currentPID,
		pageID>>d.PageSizeAsPowerOf2, virtualAddr, 1<<d.PageSizeAsPowerOf2)
	page := vm.NewPage()
	page.PhysicalFrameNumber = pageID >> d.PageSizeAsPowerOf2
	d.allocatedPages[d.usingGPU] = append(d.allocatedPages[d.usingGPU], page)

	chunk := new(MemoryChunk)
	chunk.Ptr = GPUPtr(virtualAddr)
	chunk.ByteSize = 1 << d.PageSizeAsPowerOf2
	chunk.Occupied = false
	d.memoryMasks[d.usingGPU] = append(d.memoryMasks[d.usingGPU], chunk)
}

func (d *Driver) isPageAllocated(pAddr uint64) bool {
	for _, p := range d.allocatedPages[d.usingGPU] {
		if p.PhysicalFrameNumber<<d.PageSizeAsPowerOf2 == pAddr {
			return true
		}
	}
	return false
}

func (d *Driver) tryAllocateWithExistingChunks(
	byteSize, alignment uint64,
) (ptr GPUPtr, ok bool) {
	chunks := d.memoryMasks[d.usingGPU]
	for i, chunk := range chunks {
		if chunk.Occupied {
			continue
		}

		nextAlignment := ((uint64(chunk.Ptr)-1)/alignment + 1) * alignment
		if nextAlignment <= uint64(chunk.Ptr)+chunk.ByteSize &&
			nextAlignment+byteSize <= uint64(chunk.Ptr)+chunk.ByteSize {

			ptr = GPUPtr(nextAlignment)
			ok = true

			d.splitChunk(i, ptr, byteSize)

			return
		}
	}

	return 0, false
}

func (d *Driver) splitChunk(chunkIndex int, ptr GPUPtr, byteSize uint64) {
	chunks := d.memoryMasks[d.usingGPU]
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
	d.memoryMasks[d.usingGPU] = newChunks
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
		Dst: dst,
		Src: src,
	}

	queue.Commands = append(queue.Commands, cmd)
}

// EnqueueMemCopyD2H registers a MemCopyD2HCommand in the queue.
func (d *Driver) EnqueueMemCopyD2H(
	queue *CommandQueue,
	dst interface{},
	src GPUPtr,
) {
	cmd := &MemCopyD2HCommand{
		Dst: dst,
		Src: src,
	}
	queue.Commands = append(queue.Commands, cmd)
}

// MemCopyH2D copies a memory from the host to a GPU device.
func (d *Driver) MemCopyH2D(dst GPUPtr, src interface{}) {
	queue := d.CreateCommandQueue()
	d.EnqueueMemCopyH2D(queue, dst, src)
	d.ExecuteAllCommands()
}

// MemCopyD2H copies a memory from a GPU device to the host
func (d *Driver) MemCopyD2H(dst interface{}, src GPUPtr) {
	queue := d.CreateCommandQueue()
	d.EnqueueMemCopyD2H(queue, dst, src)
	d.ExecuteAllCommands()
}
