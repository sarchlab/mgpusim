package driver

import (
	"log"

	"bytes"

	"encoding/binary"

	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/mem"
)

// GPUPtr is the type that represent a pointer pointing into the GPU memory
type GPUPtr uint64

// LocalPtr is a type that represent a pointer to a region in the LDS memory
type LocalPtr uint32

type MemoryMask struct {
	Chunks []*MemoryChunk
}

// InsertChunk
func (m *MemoryMask) InsertChunk(index int, chunk *MemoryChunk) {
	m.Chunks = append(m.Chunks, &MemoryChunk{0, 0, false})
	copy(m.Chunks[index+1:], m.Chunks[index:])
	m.Chunks[index] = chunk
}

// NewMemoryMask creates a MemoryMask. Argument capacity is the capacity of
// the underlying storage.
func NewMemoryMask(startAddr GPUPtr, capacity uint64) *MemoryMask {
	m := new(MemoryMask)
	m.Chunks = make([]*MemoryChunk, 0)

	chunk := &MemoryChunk{
		startAddr,
		capacity,
		false,
	}
	m.Chunks = append(m.Chunks, chunk)

	return m
}

// A MemoryChunk is a piece of allocated or free memory.
type MemoryChunk struct {
	Ptr      GPUPtr
	ByteSize uint64
	Occupied bool
}

func (d *Driver) registerStorage(
	storage *mem.Storage,
	loAddr GPUPtr,
	byteSize uint64,
) {
	mask := NewMemoryMask(loAddr, byteSize)
	d.memoryMasks = append(d.memoryMasks, mask)
}

// AllocateMemory allocates a chunk of memory of size byteSize in storage.
// It returns the pointer pointing to the newly allocated memory in the GPU
// memory space.
func (d *Driver) AllocateMemory(
	byteSize uint64,
) GPUPtr {
	mask := d.memoryMasks[d.usingGPU]

	var ptr GPUPtr
	for i, chunk := range mask.Chunks {
		if !chunk.Occupied && chunk.ByteSize >= byteSize {
			ptr = chunk.Ptr

			allocatedChunk := &MemoryChunk{ptr, byteSize, true}
			mask.InsertChunk(i, allocatedChunk)

			chunk.Ptr += GPUPtr(byteSize)
			chunk.ByteSize -= byteSize

			return ptr
		}
	}

	log.Fatalf("Cannot allocate memory")
	return 0
}

func (d *Driver) AllocateMemoryWithAlignment(
	byteSize uint64,
	alignment uint64,
) GPUPtr {
	mask := d.memoryMasks[d.usingGPU]

	//var ptr GPUPtr
	for i, chunk := range mask.Chunks {
		if !chunk.Occupied && chunk.ByteSize >= byteSize {

			ptr := (((uint64(chunk.Ptr) - 1) / alignment) + 1) * alignment
			if chunk.ByteSize-(ptr-uint64(chunk.Ptr)) < byteSize {
				continue
			}

			if ptr != uint64(chunk.Ptr) {
				firstChunk := &MemoryChunk{chunk.Ptr, ptr - uint64(chunk.Ptr), false}
				mask.InsertChunk(i, firstChunk)

				allocatedChunk := &MemoryChunk{GPUPtr(ptr), byteSize, true}
				mask.InsertChunk(i+1, allocatedChunk)

				chunk.ByteSize -= byteSize + (ptr - uint64(chunk.Ptr))
				chunk.Ptr = GPUPtr(ptr + byteSize)

			} else {
				allocatedChunk := &MemoryChunk{GPUPtr(ptr), byteSize, true}
				mask.InsertChunk(i, allocatedChunk)

				chunk.Ptr += GPUPtr(byteSize)
				chunk.ByteSize -= byteSize
			}

			return GPUPtr(ptr)
		}
	}

	log.Fatalf("Cannot allocate memory")
	return 0
}

// FreeMemory frees the memory pointed by ptr. The pointer must be allocated
// with the function AllocateMemory earlier. Error will be returned if the ptr
// provided is invalid.
func (d *Driver) FreeMemory(ptr GPUPtr) error {
	mask := d.memoryMasks[d.usingGPU]
	chunks := mask.Chunks
	for i := 0; i < len(chunks); i++ {
		if chunks[i].Ptr == ptr {
			chunks[i].Occupied = false

			if i != 0 && i != len(chunks)-1 && chunks[i-1].Occupied == false && chunks[i+1].Occupied == false {
				chunks[i-1].ByteSize += chunks[i].ByteSize + chunks[i+1].ByteSize
				mask.Chunks = append(chunks[:i], chunks[i+2:]...)
				return nil
			}

			if i != 0 && chunks[i-1].Occupied == false {
				chunks[i-1].ByteSize += chunks[i].ByteSize
				mask.Chunks = append(chunks[:i], chunks[i+1:]...)
				return nil
			}

			if i != len(chunks)-1 && chunks[i+1].Occupied == false {
				chunks[i].ByteSize += chunks[i+1].ByteSize
				mask.Chunks = append(chunks[:i+1], chunks[i+2:]...)
				return nil
			}
			return nil
		}
	}

	log.Fatalf("Invalid pointer")
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

	gpu := d.gpus[d.usingGPU].ToDriver
	start := d.engine.CurrentTime() + 1e-8
	req := gcn3.NewMemCopyH2DReq(start, d.ToGPUs, gpu, buffer.Bytes(), uint64(ptr))
	d.ToGPUs.Send(req)
	d.engine.Run()
	end := d.engine.CurrentTime()
	log.Printf("Memcpy H2D: [%.012f - %.012f]\n", start, end)
}

// MemoryCopyDeviceToHost copies a memory from a GPU device to the host
func (d *Driver) MemoryCopyDeviceToHost(data interface{}, ptr GPUPtr) {
	rawData := make([]byte, binary.Size(data))

	gpu := d.gpus[d.usingGPU].ToDriver
	start := d.engine.CurrentTime() + 1e-8
	req := gcn3.NewMemCopyD2HReq(start, d.ToGPUs, gpu, uint64(ptr), rawData)
	d.ToGPUs.Send(req)
	d.engine.Run()
	end := d.engine.CurrentTime()
	log.Printf("Memcpy D2H: [%.012f - %.012f]\n", start, end)

	buf := bytes.NewReader(rawData)
	binary.Read(buf, binary.LittleEndian, data)
}
