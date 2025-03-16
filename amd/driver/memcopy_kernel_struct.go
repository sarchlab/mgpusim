package driver

// KernelMemCopyArgs is the kernel struct for MemCopyD2D
type KernelMemCopyArgs struct {
	Src Ptr
	Dst Ptr
	N   int64
}
