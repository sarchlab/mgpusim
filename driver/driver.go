package driver

import (
	"log"

	"gitlab.com/yaotsu/mem"
)

// GPUPtr is the type that represent a pointer pointing into the GPU memory
type GPUPtr uint64

type MemoryMask struct {
	Chunks []*MemoryChunk
}

// InsertChunk
func (m *MemoryMask) InsertChunk(index int, chunk *MemoryChunk) {
	m.Chunks = append(m.Chunks, &MemoryChunk{0, 0, false})
	copy(m.Chunks[index+1:], m.Chunks[index:])
	m.Chunks[index] = chunk
}

// NewMemoryMask creates a MemoryMask. Argument capacity is the capacity of the underlying storage.
func NewMemoryMask(capacity uint64) *MemoryMask {
	m := new(MemoryMask)
	m.Chunks = make([]*MemoryChunk, 0)

	chunk := &MemoryChunk{0, capacity, false}
	m.Chunks = append(m.Chunks, chunk)

	return m
}

// A MemoryChunk is a piece of allocated or free memory.
type MemoryChunk struct {
	Ptr      GPUPtr
	ByteSize uint64
	Occupied bool
}

// Driver is the element that controls the GPUs under simulation
type Driver struct {
	memoryMasks map[*mem.Storage]*MemoryMask
}

// NewDriver creates a new driver
func NewDriver() *Driver {
	driver := new(Driver)

	driver.memoryMasks = make(map[*mem.Storage]*MemoryMask)

	return driver
}

// AllocateMemory allocates a chunk of memory of size byteSize in storage.
// It returns the pointer pointing to the newly allocated memory in the GPU
// memory space.
func (d *Driver) AllocateMemory(storage *mem.Storage, byteSize uint64) GPUPtr {
	mask, ok := d.memoryMasks[storage]
	if !ok {
		// TODO: Read capacity from storage
		mask = NewMemoryMask(4 * mem.GB)
		d.memoryMasks[storage] = mask
	}

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

// FreeMemory frees the memory pointed by ptr. The pointer must be allocated
// with the function AllocateMemory earlier. Error will be returned if the ptr
// provided is invalid.
func (d *Driver) FreeMemory(ptr GPUPtr) error {
	return nil
}
