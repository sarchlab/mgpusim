package gputensor

import "github.com/sarchlab/mgpusim/v4/amd/driver"

// CDNA3HiddenArgs contains the hidden kernel arguments required by CDNA3
// (gfx942) HSACOs. These must follow the explicit kernel arguments.
// The hidden block occupies 66 bytes of data at specific offsets with
// 16 bytes of padding between remainder and global offset fields.
// CDNA3HiddenArgs contains the hidden kernel arguments required by CDNA3
// (gfx942) HSACOs. Fields must be exported for reflect-based serialization
// in the driver's prepareLocalMemory.
type CDNA3HiddenArgs struct {
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad                 [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

func newCDNA3HiddenArgs(
	globalSize [3]uint32,
	localSize [3]uint16,
) CDNA3HiddenArgs {
	h := CDNA3HiddenArgs{
		HiddenGroupSizeX: localSize[0],
		HiddenGroupSizeY: localSize[1],
		HiddenGroupSizeZ: localSize[2],
	}

	for i := 0; i < 3; i++ {
		g := globalSize[i]
		l := uint32(localSize[i])
		bc := (g + l - 1) / l
		rem := g % l

		switch i {
		case 0:
			h.HiddenBlockCountX = bc
			h.HiddenRemainderX = uint16(rem)
		case 1:
			h.HiddenBlockCountY = bc
			h.HiddenRemainderY = uint16(rem)
		case 2:
			h.HiddenBlockCountZ = bc
			h.HiddenRemainderZ = uint16(rem)
		}
	}

	dims := uint16(0)
	if globalSize[0] > 1 {
		dims = 1
	}
	if globalSize[1] > 1 {
		dims = 2
	}
	if globalSize[2] > 1 {
		dims = 3
	}
	if dims == 0 {
		dims = 1
	}
	h.HiddenGridDims = dims

	return h
}

// repeat kernel: 280 bytes total
type cdna3RepeatArgs struct {
	Output   driver.Ptr // offset 0
	Input    driver.Ptr // offset 8
	InputLen uint32     // offset 16
	OutLen   uint32     // offset 20
	CDNA3HiddenArgs     // offset 24
}

// transpose_tensor kernel: 320 bytes total
type cdna3TransposeKernelArgs struct {
	In          driver.Ptr // offset 0
	Out         driver.Ptr // offset 8
	InSize      driver.Ptr // offset 16
	OutSize     driver.Ptr // offset 24
	Order       driver.Ptr // offset 32
	InIndexBuf  driver.Ptr // offset 40
	OutIndexBuf driver.Ptr // offset 48
	Dim         int32      // offset 56
	Pad0        uint32     // offset 60
	CDNA3HiddenArgs        // offset 64
}

// rotate_tensor kernel: 312 bytes total
type cdna3RotateKernelArgs struct {
	In          driver.Ptr // offset 0
	Out         driver.Ptr // offset 8
	InSize      driver.Ptr // offset 16
	OutSize     driver.Ptr // offset 24
	InIndexBuf  driver.Ptr // offset 32
	OutIndexBuf driver.Ptr // offset 40
	Dim         int32      // offset 48
	Pad0        uint32     // offset 52
	CDNA3HiddenArgs        // offset 56
}

// dilate_tensor kernel: 320 bytes total
type cdna3DilateKernelArgs struct {
	In          driver.Ptr // offset 0
	Out         driver.Ptr // offset 8
	InSize      driver.Ptr // offset 16
	OutSize     driver.Ptr // offset 24
	Dilate      driver.Ptr // offset 32
	InIndexBuf  driver.Ptr // offset 40
	OutIndexBuf driver.Ptr // offset 48
	Dim         int32      // offset 56
	Pad0        uint32     // offset 60
	CDNA3HiddenArgs        // offset 64
}

// softmax_exp kernel: 280 bytes total
type cdna3SoftmaxExpArgs struct {
	Input  driver.Ptr // offset 0
	Output driver.Ptr // offset 8
	N      int32      // offset 16
	Pad0   uint32     // offset 20
	CDNA3HiddenArgs   // offset 24
}

// softmax_div kernel: 288 bytes total
type cdna3SoftmaxDivArgs struct {
	ExpInput    driver.Ptr // offset 0
	Out         driver.Ptr // offset 8
	Denominator driver.Ptr // offset 16
	NumElement  int32      // offset 24
	BatchSize   int32      // offset 28
	CDNA3HiddenArgs        // offset 32
}

// sum_one_axis kernel: 312 bytes total
type cdna3SumOneAxisKernelArgs struct {
	In          driver.Ptr // offset 0
	Out         driver.Ptr // offset 8
	InSize      driver.Ptr // offset 16
	OutSize     driver.Ptr // offset 24
	InDim       int32      // offset 32
	Axis        int32      // offset 36
	InIndexBuf  driver.Ptr // offset 40
	OutIndexBuf driver.Ptr // offset 48
	CDNA3HiddenArgs        // offset 56
}

// scaleAdd kernel: 296 bytes total
type cdna3ScaleAddArgs struct {
	Out   driver.Ptr // offset 0
	In1   driver.Ptr // offset 8
	In2   driver.Ptr // offset 16
	Alpha float32    // offset 24
	Beta  float32    // offset 28
	N     int32      // offset 32
	Pad0  uint32     // offset 36
	CDNA3HiddenArgs  // offset 40
}

// mul kernel: 288 bytes total
type cdna3MulArgs struct {
	Out  driver.Ptr // offset 0
	In1  driver.Ptr // offset 8
	In2  driver.Ptr // offset 16
	N    int32      // offset 24
	Pad0 uint32     // offset 28
	CDNA3HiddenArgs // offset 32
}

// rmsProp kernel: 296 bytes total
type cdna3RmsPropArgs struct {
	Params       driver.Ptr // offset 0
	Gradients    driver.Ptr // offset 8
	SHistory     driver.Ptr // offset 16
	SmoothFactor float32    // offset 24
	LearningRate float32    // offset 28
	N            int32      // offset 32
	Pad0         uint32     // offset 36
	CDNA3HiddenArgs         // offset 40
}

// adam kernel: 304 bytes total
type cdna3AdamArgs struct {
	Params        driver.Ptr // offset 0
	Gradients     driver.Ptr // offset 8
	SHistory      driver.Ptr // offset 16
	VHistory      driver.Ptr // offset 24
	SmoothFactor1 float32    // offset 32
	SmoothFactor2 float32    // offset 36
	LearningRate  float32    // offset 40
	N             int32      // offset 44
	CDNA3HiddenArgs          // offset 48
}

// reluForward kernel: 280 bytes total
type cdna3ReluForwardArgs struct {
	In    driver.Ptr // offset 0
	Out   driver.Ptr // offset 8
	Count int32      // offset 16
	Pad0  uint32     // offset 20
	CDNA3HiddenArgs  // offset 24
}

// reluBackward kernel: 288 bytes total
type cdna3ReluBackwardArgs struct {
	In     driver.Ptr // offset 0
	BackIn driver.Ptr // offset 8
	Out    driver.Ptr // offset 16
	Count  int32      // offset 24
	Pad0   uint32     // offset 28
	CDNA3HiddenArgs   // offset 32
}

// im2col_2d kernel: 320 bytes total
type cdna3Im2ColKernelArg struct {
	Input    driver.Ptr // offset 0
	Output   driver.Ptr // offset 8
	InputDim [2]uint32  // offset 16
	MaskDim  [2]uint32  // offset 24
	Stride   [2]uint32  // offset 32
	PadArg   [2]uint32  // offset 40 (kernel's pad parameter)
	Dilation [2]uint32  // offset 48
	Channel  uint32     // offset 56
	Batch    uint32     // offset 60
	CDNA3HiddenArgs     // offset 64
}

// gemm_old kernel: 312 bytes total
type cdna3GemmKernArgs struct {
	M     int32      // offset 0
	N     int32      // offset 4
	K     int32      // offset 8
	Alpha float32    // offset 12
	Beta  float32    // offset 16
	Pad0  uint32     // offset 20 (align ptr to 8)
	A     driver.Ptr // offset 24
	B     driver.Ptr // offset 32
	C     driver.Ptr // offset 40
	D     driver.Ptr // offset 48
	CDNA3HiddenArgs   // offset 56
}

// MaxPoolForward kernel: 336 bytes total
type cdna3MaxPoolingForwardKernelArgs struct {
	Nthreads     int32      // offset 0
	Pad0         uint32     // offset 4
	BottomData   driver.Ptr // offset 8
	Num          int32      // offset 16
	Channels     int32      // offset 20
	Height       int32      // offset 24
	Width        int32      // offset 28
	PooledHeight int32      // offset 32
	PooledWidth  int32      // offset 36
	KernelH      int32      // offset 40
	KernelW      int32      // offset 44
	StrideH      int32      // offset 48
	StrideW      int32      // offset 52
	PadH         int32      // offset 56
	PadW         int32      // offset 60
	TopData      driver.Ptr // offset 64
	MaskData     driver.Ptr // offset 72
	CDNA3HiddenArgs         // offset 80
}

// MaxPoolBackward kernel: 336 bytes total
type cdna3MaxPoolingBackwardKernelArgs struct {
	Nthreads     int32      // offset 0
	Pad0         uint32     // offset 4
	TopDiff      driver.Ptr // offset 8
	TopMask      driver.Ptr // offset 16
	Num          int32      // offset 24
	Channels     int32      // offset 28
	Height       int32      // offset 32
	Width        int32      // offset 36
	PooledHeight int32      // offset 40
	PooledWidth  int32      // offset 44
	KernelH      int32      // offset 48
	KernelW      int32      // offset 52
	StrideH      int32      // offset 56
	StrideW      int32      // offset 60
	PadH         int32      // offset 64
	PadW         int32      // offset 68
	BottomDiff   driver.Ptr // offset 72
	CDNA3HiddenArgs         // offset 80
}

// AvgPoolForward kernel: 328 bytes total
type cdna3AvgPoolingForwardKernelArgs struct {
	Nthreads     int32      // offset 0
	Pad0         uint32     // offset 4
	BottomData   driver.Ptr // offset 8
	Num          int32      // offset 16
	Channels     int32      // offset 20
	Height       int32      // offset 24
	Width        int32      // offset 28
	PooledHeight int32      // offset 32
	PooledWidth  int32      // offset 36
	KernelH      int32      // offset 40
	KernelW      int32      // offset 44
	StrideH      int32      // offset 48
	StrideW      int32      // offset 52
	PadH         int32      // offset 56
	PadW         int32      // offset 60
	TopData      driver.Ptr // offset 64
	CDNA3HiddenArgs         // offset 72
}

// AvgPoolBackward kernel: 328 bytes total
type cdna3AvgPoolingBackwardKernelArgs struct {
	Nthreads     int32      // offset 0
	Pad0         uint32     // offset 4
	TopDiff      driver.Ptr // offset 8
	Num          int32      // offset 16
	Channels     int32      // offset 20
	Height       int32      // offset 24
	Width        int32      // offset 28
	PooledHeight int32      // offset 32
	PooledWidth  int32      // offset 36
	KernelH      int32      // offset 40
	KernelW      int32      // offset 44
	StrideH      int32      // offset 48
	StrideW      int32      // offset 52
	PadH         int32      // offset 56
	PadW         int32      // offset 60
	BottomDiff   driver.Ptr // offset 64
	CDNA3HiddenArgs         // offset 72
}

// cross_entropy_derivative / softmax_cross_entropy_derivative: 288 bytes
type cdna3CrossEntropyDerivativeArgs struct {
	Output      driver.Ptr // offset 0
	Input       driver.Ptr // offset 8
	Label       driver.Ptr // offset 16
	BatchSize   int32      // offset 24
	NumPerImage int32      // offset 28
	CDNA3HiddenArgs        // offset 32
}
