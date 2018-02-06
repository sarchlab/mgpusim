package driver

import "gitlab.com/yaotsu/mem"

// GPUPtr is the type that represent a pointer pointing into the GPU memory
type GPUPtr uint64

// Driver is the element that controlls the GPUs under simulation
type Driver struct {
}

// NewDriver creates a new driver
func NewDriver() *Driver {
	driver := new(Driver)
	return driver
}

// AllocateMemory allocates a chunk of memory of size byteSize in storage.
// It returns the pointer pointing to the newly allocated memory in the GPU
// memory space.
func (d *Driver) AllocateMemory(storage *mem.Storage, byteSize uint64) GPUPtr {
	return 0
}

// FreeMemory frees the memory pointed by ptr. The pointer must be allocated
// with the function AllocateMemory eariler. Error will be returned if the ptr
// provided is invalid.
func (d *Driver) FreeMemory(ptr GPUPtr) error {
	return nil
}
