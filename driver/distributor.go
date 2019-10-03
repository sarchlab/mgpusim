package driver

import "gitlab.com/akita/gcn3/driver/internal"

// A distributor can distribute a virtually consecutive memory to multiple GPUs.
type distributor interface {
	Distribute(
		ctx *Context,
		addr, byteSize uint64,
		gpuIDs []int,
	) (byteAllocatedOnEachGPU []uint64)
}

type distributorImpl struct {
	pageSizeAsPowerOf2 uint64
	memAllocator       internal.MemoryAllocator
}

func newDistributorImpl(memAllocator internal.MemoryAllocator) *distributorImpl {
	return &distributorImpl{
		memAllocator: memAllocator,
	}
}

func (d *distributorImpl) Distribute(
	ctx *Context,
	addr, byteSize uint64,
	gpuIDs []int,
) (byteAllocatedOnEachGPU []uint64) {
	pageSize := uint64(1 << d.pageSizeAsPowerOf2)
	if addr%pageSize != 0 {
		panic("Address much align with pages")
	}

	byteAllocatedOnEachGPU = make([]uint64, len(gpuIDs))
	numPages := ((byteSize-1)/pageSize + 1)
	numGPUs := uint64(len(gpuIDs))
	numPagesPerGPU := numPages / numGPUs
	numGPUsToUse := uint64(0)
	if numPagesPerGPU > 0 {
		numGPUsToUse = numPages / numPagesPerGPU
	}
	if numGPUsToUse > numGPUs {
		numGPUsToUse = numGPUs
	}
	remainingPages := numPages % numGPUs

	var i uint64
	var lastAllocatedGPU uint64
	for i = 0; i < numGPUsToUse; i++ {
		d.memAllocator.Remap(
			ctx.pid,
			addr+i*numPagesPerGPU*pageSize,
			numPagesPerGPU*pageSize,
			gpuIDs[i],
		)
		byteAllocatedOnEachGPU[i] += numPagesPerGPU * pageSize
		lastAllocatedGPU = i
	}

	for i := uint64(0); i < remainingPages; i++ {
		d.memAllocator.Remap(
			ctx.pid,
			addr+(numPagesPerGPU*numGPUsToUse+i)*pageSize,
			pageSize,
			gpuIDs[lastAllocatedGPU],
		)
		byteAllocatedOnEachGPU[lastAllocatedGPU] += pageSize
	}

	return
}
