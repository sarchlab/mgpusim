package driver

import (
	"log"

	"gitlab.com/akita/mem/vm"

	"bytes"

	"encoding/binary"

	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/mem"
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
	storage *mem.Storage,
) {
	d.memoryMasks = append(d.memoryMasks, make([]*MemoryChunk, 0))
	d.allocatedPages = append(d.allocatedPages, make([]*vm.Page, 0))

	d.initialAddresses = append(d.initialAddresses,
		d.totalStorageByteSize)
	d.storageSizes = append(d.storageSizes, storage.Capacity)
	d.totalStorageByteSize += storage.Capacity
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

func (d *Driver) AllocateMemoryWithAlignment(
	byteSize uint64,
	alignment uint64,
) GPUPtr {
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

func (d *Driver) allocatePage() {
	pageID := d.initialAddresses[d.usingGPU]
	for pageID < d.initialAddresses[d.usingGPU]+d.storageSizes[d.usingGPU] {
		pageIDAllocated := false
		for _, p := range d.allocatedPages[d.usingGPU] {
			if p.PhysicalFrameNumber == pageID {
				pageIDAllocated = true
				pageID += 1 << d.PageSizeAsPowerOf2
				break
			}
		}

		if !pageIDAllocated {
			break
		}
	}

	virtualAddr := pageID + 0x100000000
	d.mmu.CreatePage(d.currentPID,
		pageID, virtualAddr, 1<<d.PageSizeAsPowerOf2)
	page := vm.NewPage()
	page.PhysicalFrameNumber = pageID
	d.allocatedPages[d.usingGPU] = append(d.allocatedPages[d.usingGPU], page)

	chunk := new(MemoryChunk)
	chunk.Ptr = GPUPtr(virtualAddr)
	chunk.ByteSize = 1 << d.PageSizeAsPowerOf2
	chunk.Occupied = false
	d.memoryMasks[d.usingGPU] = append(d.memoryMasks[d.usingGPU], chunk)
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

// MemoryCopyHostToDevice copies a memory from the host to a GPU device.
func (d *Driver) MemoryCopyHostToDevice(ptr GPUPtr, data interface{}) {
	rawData := make([]byte, 0)
	buffer := bytes.NewBuffer(rawData)

	err := binary.Write(buffer, binary.LittleEndian, data)
	if err != nil {
		log.Fatal(err)
	}

	physicalAddr, found :=
		d.mmu.Translate(uint64(ptr), d.currentPID, 1<<d.PageSizeAsPowerOf2)
	if !found {
		log.Panic("failed to translate physical address")
	}

	gpu := d.gpus[d.usingGPU].ToDriver
	start := d.engine.CurrentTime() + 1e-8
	req := gcn3.NewMemCopyH2DReq(start, d.ToGPUs, gpu,
		buffer.Bytes(), uint64(physicalAddr))
	d.ToGPUs.Send(req)
	d.engine.Run()
	end := d.engine.CurrentTime()
	log.Printf("Memcpy H2D: [%.012f - %.012f]\n", start, end)
}

// MemoryCopyDeviceToHost copies a memory from a GPU device to the host
func (d *Driver) MemoryCopyDeviceToHost(data interface{}, ptr GPUPtr) {
	rawData := make([]byte, binary.Size(data))

	physicalAddr, found :=
		d.mmu.Translate(uint64(ptr), d.currentPID, 1<<d.PageSizeAsPowerOf2)
	if !found {
		log.Panic("failed to translate physical address")
	}

	gpu := d.gpus[d.usingGPU].ToDriver
	start := d.engine.CurrentTime() + 1e-8
	req := gcn3.NewMemCopyD2HReq(start, d.ToGPUs, gpu,
		uint64(physicalAddr), rawData)
	d.ToGPUs.Send(req)
	d.engine.Run()
	end := d.engine.CurrentTime()
	log.Printf("Memcpy D2H: [%.012f - %.012f]\n", start, end)

	buf := bytes.NewReader(rawData)
	binary.Read(buf, binary.LittleEndian, data)
}
