package driver

// KernelMemCopyArgs is the kernel struct for MemCopyD2D
type KernelMemCopyArgs struct {
	Src GPUPtr
	Dst GPUPtr
	N   int64
}
