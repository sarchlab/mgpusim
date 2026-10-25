package driver

import (
	_ "embed"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

//go:embed memcopy.hsaco
var kernelBytes []byte

// EnqueueMemCopyD2D uses the device copy kernel on this stream.
func (r *Runtime) EnqueueMemCopyD2D(q CommandQueue, dst, src Ptr, size int) CommandID {
	co := insts.LoadKernelCodeObjectFromBytes(kernelBytes, "copyKernel")
	if co == nil {
		panic("fail to load copyKernel")
	}
	args := KernelMemCopyArgs{src, dst, int64(size)}
	return r.EnqueueLaunchKernel(q, co, [3]uint32{uint32((size + 3) / 4), 1, 1}, [3]uint16{64, 1, 1}, &args)
}
