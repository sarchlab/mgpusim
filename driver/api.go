package driver

import (
	"log"
	"sync/atomic"

	"github.com/rs/xid"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/driver/internal"
	"gitlab.com/akita/util/ca"
)

var nextPID uint64

// Init creates a context to be used in the following API calls.
func (d *Driver) Init() *Context {
	atomic.AddUint64(&nextPID, 1)

	c := &Context{
		pid:          ca.PID(nextPID),
		currentGPUID: 1,
	}

	d.contextMutex.Lock()
	d.contexts = append(d.contexts, c)
	d.contextMutex.Unlock()

	return c
}

// GetNumGPUs return the number of GPUs in the platform
func (d *Driver) GetNumGPUs() int {
	return len(d.GPUs)
}

// SelectGPU requires the driver to perform the following APIs on a selected
// GPU
func (d *Driver) SelectGPU(c *Context, gpuID int) {
	if gpuID >= len(d.devices) {
		log.Panicf("GPU %d is not available", gpuID)
	}
	c.currentGPUID = gpuID
}

// CreateUnifiedGPU can create a virtual GPU that bundles multiple GPUs
// together. It returns the DeviceID of the created unified multi-GPU device.
func (d *Driver) CreateUnifiedGPU(c *Context, gpuIDs []int) int {
	d.mustNotBeAnEmptyList(gpuIDs)
	d.mustBeAllActualGPUs(gpuIDs)

	dev := &internal.Device{
		ID:            len(d.devices),
		Type:          internal.DeviceTypeUnifiedGPU,
		UnifiedGPUIDs: gpuIDs,
		Log2PageSize:  d.Log2PageSize,
	}

	for _, gpuID := range gpuIDs {
		dev.ActualGPUs = append(dev.ActualGPUs, d.devices[gpuID])
	}

	d.devices = append(d.devices, dev)
	d.memAllocator.RegisterDevice(dev)

	return dev.ID
}

func (d *Driver) mustNotBeAnEmptyList(gpuIDs []int) {
	if len(gpuIDs) == 0 {
		panic("must unify at least 1 GPU")
	}
}

func (d *Driver) mustBeAllActualGPUs(gpuIDs []int) {
	for _, gpuID := range gpuIDs {
		dev := d.devices[gpuID]
		if dev.Type != internal.DeviceTypeGPU {
			panic("can only unify GPUs")
		}
	}
}

// CreateCommandQueue creates a command queue in the driver
func (d *Driver) CreateCommandQueue(c *Context) *CommandQueue {
	q := new(CommandQueue)
	q.GPUID = c.currentGPUID
	q.Context = c

	c.queueMutex.Lock()
	c.queues = append(c.queues, q)
	c.queueMutex.Unlock()

	return q
}

// DrainCommandQueue will return when there is no command to execute
func (d *Driver) DrainCommandQueue(q *CommandQueue) {
	listener := q.Subscribe()
	defer q.Unsubscribe(listener)

	d.enqueueSignal <- true

	for {
		if q.NumCommand() == 0 {
			return
		}
		listener.Wait()
	}
}

// AllocateMemory allocates a chunk of memory of size byteSize in storage.
// It returns the pointer pointing to the newly allocated memory in the GPU
// memory space.
func (d *Driver) AllocateMemory(
	ctx *Context,
	byteSize uint64,
) GPUPtr {
	ptr := d.memAllocator.Allocate(ctx.pid, byteSize, ctx.currentGPUID)
	return GPUPtr(ptr)
}

//AllocateUnifiedMemory allocates a unified memory. Allocation is done on CPU
func (d *Driver) AllocateUnifiedMemory(
	ctx *Context,
	byteSize uint64,
) GPUPtr {
	return GPUPtr(d.memAllocator.AllocateUnified(ctx.pid, byteSize))
}

// Remap keeps the virtual address unchanged and moves the physical address to
// another GPU
func (d *Driver) Remap(ctx *Context, addr, size uint64, deviceID int) {
	d.memAllocator.Remap(ctx.pid, addr, size, deviceID)
}

// Distribute rearranges a consecutive virtual memory space and re-allocate the
// memory on designated GPUs. This function returns the number of bytes
// allocated to each GPU.
func (d *Driver) Distribute(
	ctx *Context,
	addr GPUPtr,
	byteSize uint64,
	gpuIDs []int,
) []uint64 {
	return d.distributor.Distribute(ctx, uint64(addr), byteSize, gpuIDs)
}

// FreeMemory frees the memory pointed by ptr. The pointer must be allocated
// with the function AllocateMemory earlier. Error will be returned if the ptr
// provided is invalid.
func (d *Driver) FreeMemory(ctx *Context, ptr GPUPtr) error {
	d.memAllocator.Free(uint64(ptr))
	return nil
}

// EnqueueMemCopyH2D registers a MemCopyH2DCommand in the queue.
func (d *Driver) EnqueueMemCopyH2D(
	queue *CommandQueue,
	dst GPUPtr,
	src interface{},
) {
	d.enqueueFlushBeforeMemCopy(queue)
	cmd := &MemCopyH2DCommand{
		ID:  xid.New().String(),
		Dst: dst,
		Src: src,
	}
	d.Enqueue(queue, cmd)
}

// EnqueueMemCopyD2H registers a MemCopyD2HCommand in the queue.
func (d *Driver) EnqueueMemCopyD2H(
	queue *CommandQueue,
	dst interface{},
	src GPUPtr,
) {
	d.enqueueFlushBeforeMemCopy(queue)
	cmd := &MemCopyD2HCommand{
		ID:  xid.New().String(),
		Dst: dst,
		Src: src,
	}
	d.Enqueue(queue, cmd)
}

func (d *Driver) enqueueFlushBeforeMemCopy(queue *CommandQueue) {
	dirty := queue.Context.l2Dirty
	queue.commandsMutex.Lock()
	for _, cmd := range queue.commands {
		switch cmd.(type) {
		case *LaunchKernelCommand:
			dirty = true
		case *FlushCommand:
			dirty = false
		}
	}
	queue.commandsMutex.Unlock()

	if dirty {
		d.enqueueFinalFlush(queue)
	}
}

func (d *Driver) enqueueFinalFlush(queue *CommandQueue) {
	cmd := &FlushCommand{
		ID: akita.GetIDGenerator().Generate(),
	}
	d.Enqueue(queue, cmd)
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
